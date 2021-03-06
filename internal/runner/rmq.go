// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This contains the implementation of a RabbitMQ (rmq) client that will
// be used to retrieve work from RMQ and to query RMQ for extant queues
// within an StudioML Exchange

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	rh "github.com/michaelklishin/rabbit-hole"

	"github.com/rs/xid"
	"github.com/streadway/amqp"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// RabbitMQ encapsulated the configuration and extant extant client for a
// queue server
//
type RabbitMQ struct {
	url       *url.URL // amqp URL to be used for the rmq Server
	Identity  string   // A URL stripped of the user name and password, making it safe for logging etc
	exchange  string
	mgmt      *url.URL        // URL for the management interface on the rmq
	host      string          // The hostname that was specified for the RMQ server
	user      string          // user name for the management interface on rmq
	pass      string          // password for the management interface on rmq
	transport *http.Transport // Custom transport to allow for connections to be actively closed
	wrapper   *Wrapper        // Decryption infoprmation for messages with encrypted payloads
}

// DefaultStudioRMQExchange is the topic name used within RabbitMQ for StudioML based message queuing
const DefaultStudioRMQExchange = "StudioML.topic"

// NewRabbitMQ takes the uri identifing a server and will configure the client
// data structure needed to call methods against the server
//
// The order of these two parameters needs to reflect key, value pair that
// the GetKnown function returns
//
func NewRabbitMQ(uri string, creds string, wrapper *Wrapper) (rmq *RabbitMQ, err kv.Error) {

	ampq, errGo := url.Parse(os.ExpandEnv(uri))
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", os.ExpandEnv(uri))
	}

	rmq = &RabbitMQ{
		// "amqp://guest:guest@localhost:5672/%2F?connection_attempts=50",
		// "http://127.0.0.1:15672",
		exchange: DefaultStudioRMQExchange,
		user:     "guest",
		pass:     "guest",
		host:     ampq.Hostname(),
		wrapper:  wrapper,
	}

	// The Path will have a vhost that has been escaped.  The identity does not require a valid URL just a unique
	// label
	ampq.Path, _ = url.PathUnescape(ampq.Path)
	ampq.User = nil
	ampq.RawQuery = ""
	ampq.Fragment = ""
	rmq.Identity = ampq.String()

	userPass := strings.Split(creds, ":")
	if len(userPass) != 2 {
		return nil, kv.NewError("Username password missing or malformed").With("stack", stack.Trace().TrimRuntime()).With("creds", creds, "uri", ampq.String())
	}
	ampq.User = url.UserPassword(userPass[0], userPass[1])

	// Update the fully qualified URL with the credentials
	rmq.url = ampq

	rmq.user = userPass[0]
	rmq.pass = userPass[1]
	rmq.mgmt = &url.URL{
		Scheme: "http",
		User:   url.UserPassword(userPass[0], userPass[1]),
		Host:   fmt.Sprintf("%s:%d", rmq.host, 15672),
	}

	return rmq, nil
}

func (rmq *RabbitMQ) attachQ() (conn *amqp.Connection, ch *amqp.Channel, err kv.Error) {

	conn, errGo := amqp.Dial(rmq.url.String())
	if errGo != nil {
		return nil, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.Identity)
	}

	if ch, errGo = conn.Channel(); errGo != nil {
		return nil, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.Identity)
	}

	if errGo := ch.ExchangeDeclare(rmq.exchange, "topic", true, true, false, false, nil); errGo != nil {
		return nil, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.Identity).With("exchange", rmq.exchange)
	}
	return conn, ch, nil
}

func (rmq *RabbitMQ) attachMgmt(timeout time.Duration) (mgmt *rh.Client, err kv.Error) {
	user := rmq.mgmt.User.Username()
	pass, _ := rmq.mgmt.User.Password()

	mgmt, errGo := rh.NewClient(rmq.mgmt.String(), user, pass)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("user", user).With("uri", rmq.mgmt).With("exchange", rmq.exchange)
	}

	if rmq.transport == nil {
		rmq.transport = &http.Transport{
			MaxIdleConns:    1,
			IdleConnTimeout: timeout,
		}
	}
	mgmt.SetTransport(rmq.transport)

	return mgmt, nil
}

// Refresh will examine the RMQ exchange a extract a list of the queues that relate to
// StudioML work from the rmq exchange.
//
func (rmq *RabbitMQ) Refresh(ctx context.Context, matcher *regexp.Regexp, mismatcher *regexp.Regexp) (known map[string]interface{}, err kv.Error) {

	timeout := time.Duration(time.Minute)
	if deadline, isPresent := ctx.Deadline(); isPresent {
		timeout = time.Until(deadline)
	}

	known = map[string]interface{}{}

	mgmt, err := rmq.attachMgmt(timeout)
	if err != nil {
		return known, err
	}
	defer func() {
		rmq.transport.CloseIdleConnections()
	}()

	binds, errGo := mgmt.ListBindings()
	if errGo != nil {
		return known, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.mgmt)
	}

	for _, b := range binds {
		if b.Source == DefaultStudioRMQExchange && strings.HasPrefix(b.RoutingKey, "StudioML.") {
			// Make sure any retrieved Q names match the caller supplied regular expression
			if matcher != nil {
				if !matcher.MatchString(b.Destination) {
					continue
				}
			}
			if mismatcher != nil {
				// We cannot allow an excluded queue
				if mismatcher.MatchString(b.Destination) {
					continue
				}
			}
			queue := fmt.Sprintf("%s?%s", url.PathEscape(b.Vhost), url.PathEscape(b.Destination))
			known[queue] = b.Vhost
		}
	}

	return known, nil
}

// GetKnown will connect to the rabbitMQ server identified in the receiver, rmq, and will
// query it for any queues that match the matcher regular expression
//
// found contains a map of keys that have an uncredentialed URL, and the value which is the user name and password for the URL
//
// The URL path is going to be the vhost and the queue name
//
func (rmq *RabbitMQ) GetKnown(ctx context.Context, matcher *regexp.Regexp, mismatcher *regexp.Regexp) (found map[string]string, err kv.Error) {
	known, err := rmq.Refresh(ctx, matcher, mismatcher)
	if err != nil {
		return nil, err
	}

	creds := rmq.user + ":" + rmq.pass

	// Construct the found queue reference prefix
	qURL := rmq.url
	rmq.url.User = nil
	qURL.RawQuery = ""

	found = make(map[string]string, len(known))

	for hostQueue := range known {
		// Copy the credentials into the value portion of the returned collection
		// and the uncredentialed URL and queue name into the key portion
		found[qURL.String()+"?"+strings.TrimPrefix(hostQueue, "%2F?")] = creds
	}
	return found, nil
}

// Exists will connect to the rabbitMQ server identified in the receiver, rmq, and will
// query it to see if the queue identified by the studio go runner subscription exists
//
func (rmq *RabbitMQ) Exists(ctx context.Context, subscription string) (exists bool, err kv.Error) {
	destHost := strings.Split(subscription, "?")
	if len(destHost) != 2 {
		return false, kv.NewError("subscription supplied was not question-mark separated").With("stack", stack.Trace().TrimRuntime()).With("subscription", subscription)
	}

	vhost, errGo := url.PathUnescape(destHost[0])
	if errGo != nil {
		return false, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("subscription", subscription).With("vhost", destHost[0])
	}
	queue, errGo := url.PathUnescape(destHost[1])
	if errGo != nil {
		return false, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("subscription", subscription).With("queue", destHost[1])
	}

	mgmt, err := rmq.attachMgmt(15 * time.Second)
	if err != nil {
		return false, err
	}
	defer func() {
		rmq.transport.CloseIdleConnections()
	}()

	if _, errGo = mgmt.GetQueue(vhost, queue); errGo != nil {
		if response, ok := errGo.(rh.ErrorResponse); ok && response.StatusCode == 404 {
			return false, nil
		}
		return false, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("uri", rmq.mgmt)
	}

	return true, nil
}

// Work will connect to the rabbitMQ server identified in the receiver, rmq, and will see if any work
// can be found on the queue identified by the go runner subscription and present work
// to the handler for processing
//
func (rmq *RabbitMQ) Work(ctx context.Context, qt *QueueTask) (msgProcessed bool, resource *Resource, err kv.Error) {

	splits := strings.SplitN(qt.Subscription, "?", 2)
	if len(splits) != 2 {
		return false, nil, kv.NewError("malformed rmq subscription").With("stack", stack.Trace().TrimRuntime()).With("subscription", qt.Subscription)
	}

	conn, ch, err := rmq.attachQ()
	if err != nil {
		return false, nil, err
	}
	defer func() {
		ch.Close()
		conn.Close()
	}()

	queue, errGo := url.PathUnescape(splits[1])
	if errGo != nil {
		return false, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("subscription", qt.Subscription)
	}
	queue = strings.Trim(queue, "/")

	msg, ok, errGo := ch.Get(queue, false)
	if errGo != nil {
		return false, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("queue", queue)
	}
	if !ok {
		return false, nil, nil
	}

	qt.Msg = msg.Body

	rsc, ack, err := qt.Handler(ctx, qt)
	if ack {
		resource = rsc
		if errGo := msg.Ack(false); errGo != nil {
			return false, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("subscription", qt.Subscription)
		}
	} else {
		msg.Nack(false, true)
	}

	return true, resource, err
}

// This file contains the implementation of a test subsystem
// for deploying rabbitMQ in test scenarios where it
// has been installed for the purposes of running end-to-end
// tests related to queue handling and state management

var (
	testQErr = kv.NewError("uninitialized").With("stack", stack.Trace().TrimRuntime())
	qCheck   sync.Once
)

// PingRMQServer is used to validate the a RabbitMQ server is alive and active on the administration port.
//
// amqpURL is the standard client amqp uri supplied by a caller. amqpURL will be parsed and converted into
// the administration endpoint and then tested.
//
func PingRMQServer(amqpURL string) (err kv.Error) {

	qCheck.Do(func() {

		if len(amqpURL) == 0 {
			testQErr = kv.NewError("amqpURL was not specified on the command line, or as an env var, cannot start rabbitMQ").With("stack", stack.Trace().TrimRuntime())
			return
		}

		q := os.ExpandEnv(amqpURL)

		uri, errGo := amqp.ParseURI(q)
		if errGo != nil {
			testQErr = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			return
		}
		uri.Port += 10000

		// Start by making sure that when things were started we saw a rabbitMQ configured
		// on the localhost.  If so then check that the rabbitMQ started automatically as a result of
		// the Dockerfile_standalone, or Dockerfile_workstation setup
		//
		rmqc, errGo := rh.NewClient("http://"+uri.Host+":"+strconv.Itoa(uri.Port), uri.Username, uri.Password)
		if errGo != nil {
			testQErr = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			return
		}

		rmqc.SetTransport(&http.Transport{
			ResponseHeaderTimeout: time.Duration(15 * time.Second),
		})
		rmqc.SetTimeout(time.Duration(15 * time.Second))

		// declares an exchange for the queues
		exhangeSettings := rh.ExchangeSettings{
			Type:       "topic",
			Durable:    true,
			AutoDelete: true,
		}
		if _, errGo = rmqc.DeclareExchange("/", DefaultStudioRMQExchange, exhangeSettings); errGo != nil {
			testQErr = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			return
		}

		// declares a queue
		qn := "rmq_runner_test_" + xid.New().String()
		if _, errGo = rmqc.DeclareQueue("/", qn, rh.QueueSettings{Durable: false}); errGo != nil {
			testQErr = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			return
		}

		bi := rh.BindingInfo{
			Source:          DefaultStudioRMQExchange,
			Destination:     qn,
			DestinationType: "queue",
			RoutingKey:      "StudioML." + qn,
			Arguments:       map[string]interface{}{},
		}

		if _, errGo = rmqc.DeclareBinding("/", bi); errGo != nil {
			testQErr = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			return
		}

		testQErr = nil
	})

	return testQErr
}

// QueueDeclare is a shim method for creating a queue within the rabbitMQ
// server defined by the receiver
//
func (rmq *RabbitMQ) QueueDeclare(qName string) (err kv.Error) {
	conn, ch, err := rmq.attachQ()
	if err != nil {
		return err
	}
	defer func() {
		ch.Close()
		conn.Close()
	}()

	_, errGo := ch.QueueDeclare(
		qName, // name
		false, // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("qName", qName).With("uri", rmq.mgmt).With("exchange", rmq.exchange)
	}

	if errGo = ch.QueueBind(qName, "StudioML."+qName, "StudioML.topic", false, nil); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("qName", qName).With("uri", rmq.mgmt).With("exchange", rmq.exchange)
	}

	return nil
}

// One would typically keep a channel of publishings, a sequence number, and a
// set of unacknowledged sequence numbers and loop until the publishing channel
// is closed.
func confirmOne(confirms <-chan amqp.Confirmation) {

	if confirmed := <-confirms; !confirmed.Ack {
		fmt.Println("failed delivery of delivery tag: ", confirmed, "stack", stack.Trace().TrimRuntime())
	}
}

// Publish is a shim method for tests to use for sending requeues to a queue
//
func (rmq *RabbitMQ) Publish(routingKey string, contentType string, msg []byte) (err kv.Error) {
	conn, ch, err := rmq.attachQ()
	if err != nil {
		return err
	}
	defer func() {
		ch.Close()
		conn.Close()
	}()

	errGo := ch.Confirm(false)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("routingKey", routingKey).With("uri", rmq.mgmt).With("exchange", rmq.exchange)
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	defer confirmOne(confirms)

	errGo = ch.Publish(
		rmq.exchange, // exchange
		routingKey,   // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType: contentType,
			Body:        msg,
		})
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("routingKey", routingKey).With("uri", rmq.mgmt).With("exchange", rmq.exchange)
	}
	return nil
}
