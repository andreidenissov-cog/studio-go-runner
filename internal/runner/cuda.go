// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the data structures used by the CUDA package that are used
// for when the platform is and is not supported

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
	"github.com/rs/xid"

	"github.com/lthibault/jitterbug"

	"github.com/mitchellh/copystructure"
)

type device struct {
	UUID       string    `json:"uuid"`
	Name       string    `json:"name"`
	Temp       uint      `json:"temp"`
	Powr       uint      `json:"powr"`
	MemTot     uint64    `json:"memtot"`
	MemUsed    uint64    `json:"memused"`
	MemFree    uint64    `json:"memfree"`
	EccFailure *kv.Error `json:"eccfailure"`
}

type cudaDevices struct {
	Devices []device `json:"devices"`
}

// GPUTrack is used to track usage of GPU cards and any kv.generated by the cards
// at the hardware level
//
type GPUTrack struct {
	UUID       string              // The UUID designation for the GPU being managed
	Slots      uint                // The number of logical slots the GPU based on its size has
	Mem        uint64              // The amount of memory the GPU posses
	FreeSlots  uint                // The number of free logical slots the GPU has available
	FreeMem    uint64              // The amount of free memory the GPU has
	EccFailure *kv.Error           // Any Ecc failure related error messages, nil if no kv.encountered
	Tracking   map[string]struct{} // Used to validate allocations as they are release
}

type gpuTracker struct {
	Allocs map[string]*GPUTrack
	sync.Mutex
}

var (
	// A map keyed on the nvidia device UUID containing information about cards and
	// their occupancy by the go runner.
	//
	gpuAllocs gpuTracker

	// UseGPU is used for specific types of testing to disable GPU tests when there
	// are GPU cards potentially present but they need to be disabled, this flag
	// is not used during production to change behavior in any way
	UseGPU *bool

	// CudaInitErr records the result of the CUDA library initialization that would
	// impact ongoing operation
	CudaInitErr *kv.Error

	// CudaInitWarnings records warnings and kv.that are deemed not be be fatal
	// to the ongoing CUDA library usage but are of importance
	CudaInitWarnings = []kv.Error{}

	// CudaInTest is used to check if the running process is a go test process, if so then
	// this will disable certain types of checking when using very limited GPU
	// Hardware
	CudaInTest = false
)

func init() {
	temp := true
	UseGPU = &temp

	gpuDevices, err := getCUDAInfo()
	if err != nil {
		CudaInitErr = &err
		CudaInitWarnings = append(CudaInitWarnings, err)
		return
	}

	devs := os.Getenv("CUDA_VISIBLE_DEVICES")
	if len(devs) == 0 {
		devs = os.Getenv("NVIDIA_VISIBLE_DEVICES")
	}

	visDevices := strings.Split(devs, ",")

	if devs == "all" {
		visDevices = make([]string, 0, len(gpuDevices.Devices))
		for _, device := range gpuDevices.Devices {
			visDevices = append(visDevices, device.UUID)
		}
	}

	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()
	gpuAllocs.Allocs = make(map[string]*GPUTrack, len(visDevices))

	// If the visDevices were specified use then to generate existing entries inside the device map.
	// These entries will then get filled in later.
	//
	// Look to see if we have any index values in here, it really should be all UUID strings.
	// Warn if we find some, but still continue.
	warned := false
	for _, id := range visDevices {
		if len(id) == 0 {
			continue
		}
		if i, err := strconv.Atoi(id); err == nil {
			if !warned {
				warned = true
				CudaInitWarnings = append(CudaInitWarnings, kv.NewError("CUDA_VISIBLE_DEVICES should be using UUIDs not indexes").With("stack", stack.Trace().TrimRuntime()))
			}
			if i > len(gpuDevices.Devices) {
				CudaInitWarnings = append(CudaInitWarnings, kv.NewError("CUDA_VISIBLE_DEVICES contained an index past the known population of GPU cards").With("stack", stack.Trace().TrimRuntime()))
			}
			gpuAllocs.Allocs[gpuDevices.Devices[i].UUID] = &GPUTrack{Tracking: map[string]struct{}{}}
		} else {
			gpuAllocs.Allocs[id] = &GPUTrack{Tracking: map[string]struct{}{}}
		}
	}

	if len(gpuAllocs.Allocs) == 0 {
		for _, dev := range gpuDevices.Devices {
			gpuAllocs.Allocs[dev.UUID] = &GPUTrack{Tracking: map[string]struct{}{}}
		}
	}

	// Scan the inventory, checking matches if they were specified in the visibility env var and then fill
	// in real world data
	//
	for _, dev := range gpuDevices.Devices {
		// Dont include devices that were not specified by CUDA_VISIBLE_DEVICES
		if _, isPresent := gpuAllocs.Allocs[dev.UUID]; !isPresent {
			fmt.Println("GPU Skipped", dev.UUID)
			continue
		}

		track := &GPUTrack{
			UUID:       dev.UUID,
			Mem:        dev.MemFree,
			EccFailure: dev.EccFailure,
			Tracking:   map[string]struct{}{},
		}
		switch {
		case strings.Contains(dev.Name, "GTX 1050"),
			strings.Contains(dev.Name, "GTX 1060"):
			track.Slots = 2
		case strings.Contains(dev.Name, "GTX 1070"),
			strings.Contains(dev.Name, "GTX 1080"):
			track.Slots = 2
		case strings.Contains(dev.Name, "TITAN X"):
			track.Slots = 2
		case strings.Contains(dev.Name, "RTX 2080 Ti"):
			track.Slots = 2
		case strings.Contains(dev.Name, "Tesla K80"):
			track.Slots = 2
		case strings.Contains(dev.Name, "Tesla P40"):
			track.Slots = 4
		case strings.Contains(dev.Name, "Tesla P100"):
			track.Slots = 8
		case strings.Contains(dev.Name, "Tesla V100"):
			track.Slots = 16
		default:
			CudaInitWarnings = append(CudaInitWarnings, kv.NewError("unrecognized gpu device").With("gpu_name", dev.Name).With("gpu_uuid", dev.UUID).With("stack", stack.Trace().TrimRuntime()))
		}
		track.FreeSlots = track.Slots
		track.FreeMem = track.Mem
		gpuAllocs.Allocs[dev.UUID] = track
	}
}

func GetCUDAInfo() (outDevs cudaDevices, err kv.Error) {
	return getCUDAInfo()
}

// GPUInventory can be used to extract a copy of the current state of the GPU hardware seen within the
// runner
func GPUInventory() (gpus []GPUTrack, err kv.Error) {

	gpus = []GPUTrack{}

	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	for _, alloc := range gpuAllocs.Allocs {
		cpy, errGo := copystructure.Copy(*alloc)
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		gpus = append(gpus, cpy.(GPUTrack))
	}
	return gpus, nil
}

// MonitorGPUs will having initialized all of the devices in the tracking map
// when started as a go function check the devices for ECC and other kv.marking
// failed GPUs
//
func MonitorGPUs(ctx context.Context, statusC chan<- []string, errC chan<- kv.Error) {
	// Take all of the warnings etc that were gathered during initialization and
	// get them back to the error handling listener
	for _, warn := range CudaInitWarnings {
		select {
		case errC <- warn:
		case <-time.After(time.Second):
			// last gasp attempt to output the error
			fmt.Println(warn)
		}
	}

	firstTime := true

	t := jitterbug.New(time.Second*30, &jitterbug.Norm{Stdev: time.Second * 3})
	defer t.Stop()

	for {
		select {
		case <-t.C:
			gpuDevices, err := getCUDAInfo()
			if err != nil {
				select {
				case errC <- err:
				default:
					// last gasp attempt to output the error
					fmt.Println(err)
				}
			}
			// Look at allhe GPUs we have in our hardware config
			for _, dev := range gpuDevices.Devices {
				if firstTime {
					msg := []string{"gpu found", "name", dev.Name, "uuid", dev.UUID, "stack", stack.Trace().TrimRuntime().String()}
					select {
					case statusC <- msg:
					case <-time.After(time.Second):
						fmt.Println(msg)
					}
				}
				if dev.EccFailure != nil {
					gpuAllocs.Lock()
					// Check to see if the hardware GPU had a failure
					// and if it is in the tracking table and does
					// not yet have an error logged log the error
					// in the tracking table
					if gpu, isPresent := gpuAllocs.Allocs[dev.UUID]; isPresent {
						if gpu.EccFailure == nil {
							gpu.EccFailure = dev.EccFailure
							gpuAllocs.Allocs[gpu.UUID] = gpu
						}
					}
					gpuAllocs.Unlock()
					select {
					case errC <- *dev.EccFailure:
					default:
						// last gasp attempt to output the error
						fmt.Println(dev.EccFailure)
					}
				}
			}
			firstTime = false
		case <-ctx.Done():
			return
		}
	}
}

// GPUCount returns the number of allocatable GPU resources
func GPUCount() (cnt int) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	return len(gpuAllocs.Allocs)
}

// GPUSlots gets the free and total number of GPU capacity slots within
// the machine
//
func GPUSlots() (cnt uint, freeCnt uint) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	for _, alloc := range gpuAllocs.Allocs {
		cnt += alloc.Slots
		freeCnt += alloc.FreeSlots
	}
	return cnt, freeCnt
}

// LargestFreeGPUSlots gets the largest number of single device free GPU slots
//
func LargestFreeGPUSlots() (cnt uint) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	for _, alloc := range gpuAllocs.Allocs {
		if alloc.FreeSlots > cnt {
			cnt = alloc.FreeSlots
		}
	}
	return cnt
}

// TotalFreeGPUSlots gets the largest number of single device free GPU slots
//
func TotalFreeGPUSlots() (cnt uint) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	for _, alloc := range gpuAllocs.Allocs {
		cnt += alloc.FreeSlots
	}
	return cnt
}

// LargestFreeGPUMem will obtain the largest number of available GPU slots
// on any of the individual cards accessible to the runner
func LargestFreeGPUMem() (freeMem uint64) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	for _, alloc := range gpuAllocs.Allocs {
		if alloc.Slots != 0 && alloc.FreeMem > freeMem {
			freeMem = alloc.FreeMem
		}
	}
	return freeMem
}

// GPUAllocated is used to record the allocation/reservation of a GPU resource on behalf of a caller
//
type GPUAllocated struct {
	tracking string            // Allocation tracking ID
	uuid     string            // The device identifier this allocation was successful against
	slots    uint              // The number of GPU slots given from the allocation
	mem      uint64            // The amount of memory given to the allocation
	Env      map[string]string // Any environment variables the device allocator wants the runner to use
}

// GPUAllocations records the allocations that together are present to a caller.
//
type GPUAllocations []*GPUAllocated

// AllocGPU will select the default allocation pool for GPUs and call the allocation for it.
//
func AllocGPU(maxGPU uint, maxGPUMem uint64, unitsOfAllocation []uint, live bool) (alloc GPUAllocations, err kv.Error) {
	return gpuAllocs.AllocGPU(maxGPU, maxGPUMem, unitsOfAllocation, live)
}

func evens(start int, end int) (result []int) {
	result = []int{start}
	inc := 1
	for cur := start + 1; cur < end+1; cur += inc {
		if cur%2 == 0 {
			result = append(result, cur)
			inc = 2
		}
	}
	return result
}

// AllocGPU will attempt to find a free CUDA capable GPU from a supplied allocator pool
// and assign it to the client.  It will on finding a device set the appropriate values
// in the allocated return structure that the client can use to manage their resource
// consumption to match the permitted limits.
//
// When allocations occur across multiple devices the units of allocation parameter
// defines the grainularity that the cards must conform to in terms of slots.
//
// Any allocations will take an entire card, we do not break cards across experiments
//
// This receiver uses a user supplied pool which allows for unit tests to be written that use a
// custom pool
//
// The live parameter if false can be used to test if the allocation would be successful
// without performing it.  If live false is used no allocation will be returned and err will be nil
// if the allocation have been successful.
//
func (allocator *gpuTracker) AllocGPU(maxGPU uint, maxGPUMem uint64, unitsOfAllocation []uint, live bool) (alloc GPUAllocations, err kv.Error) {

	alloc = GPUAllocations{}

	if maxGPU == 0 && maxGPUMem == 0 {
		return alloc, nil
	}

	// Start with the smallest granularity of allocations permitted and try and find a fit for the total,
	// then continue up through the granularities until we have exhausted the options

	// Put the units of allocation in to a searchable slice, putting the largest first
	units := make([]int, len(unitsOfAllocation))
	for i, unit := range unitsOfAllocation {
		units[i] = int(unit)
	}
	// If needed create an exact match definition for the case where the caller failed to
	// supply units of allocation, and also the even numbers between the minimum number
	// of slots for GPUs being 4 and the upper limit
	if len(units) == 0 {
		units = evens(2, int(maxGPU+1)*2)
	}

	sort.Slice(units, func(i, j int) bool { return units[i] < units[j] })

	// Start building logging style information to be used in the
	// event of a real error
	kvDetails := []interface{}{"maxGPU", maxGPU, "units", units}

	// Now we lock after doing initialization of the functions own variables
	allocator.Lock()
	defer allocator.Unlock()

	// Add a structure that will be used later to order our UUIDs
	// by the number of free slots they have
	type SlotsByUUID struct {
		uuid      string
		freeSlots uint
	}
	slotsByUUID := make([]SlotsByUUID, 0, len(allocator.Allocs))

	// Take any cards that have the exact number of free slots that we have
	// in our permitted units and use those, but exclude cards with
	// ECC errors
	usableAllocs := make(map[string]*GPUTrack, len(allocator.Allocs))
	for k, v := range allocator.Allocs {
		// Cannot use this cards it is broken
		if v.EccFailure != nil {
			continue
		}
		// Make sure the units contains the value of the valid range of slots
		// acceptable to the caller
		pos := sort.SearchInts(units, int(v.Slots))
		if pos < len(units) && int(v.Slots) == units[pos] {
			usableAllocs[k] = v
			slotsByUUID = append(slotsByUUID, SlotsByUUID{uuid: v.UUID, freeSlots: v.FreeSlots})
		}
	}

	if len(slotsByUUID) == 0 {
		kvDetails = append(kvDetails, []interface{}{"allocs", spew.Sdump(allocator.Allocs)}...)
		return nil, kv.NewError("insufficient free GPUs").With(kvDetails...)
	}

	// Take the permitted cards and sort their UUIDs in order of the
	// smallest number of free slots first
	sort.Slice(slotsByUUID, func(i, j int) bool {
		if slotsByUUID[i].freeSlots < slotsByUUID[j].freeSlots {
			return true
		}

		if slotsByUUID[i].freeSlots > slotsByUUID[j].freeSlots {
			return false
		}

		return slotsByUUID[i].uuid < slotsByUUID[j].uuid
	})

	kvDetails = append(kvDetails, []interface{}{"slots", slotsByUUID})

	// Because we know the preferred allocation units we can simply start with the smallest quantity
	// and if we can slowly build up enough of the smaller items to meet the need, that become one
	// combination.
	//
	type reservation struct {
		uuid  string
		slots uint
	}
	type combination struct {
		cards []reservation
		waste int
	}

	combinations := []combination{}

	// Go though building combinations that work and track the waste for each solution.
	//
	for i, uuid := range slotsByUUID {
		slotsFound := usableAllocs[uuid.uuid].FreeSlots
		cmd := combination{cards: []reservation{{uuid: uuid.uuid, slots: usableAllocs[uuid.uuid].FreeSlots}}}
		func() {
			if slotsFound < maxGPU && i < len(slotsByUUID) {
				for _, nextUUID := range slotsByUUID[i+1:] {
					slotsFound += usableAllocs[uuid.uuid].FreeSlots
					cmd.cards = append(cmd.cards, reservation{uuid: nextUUID.uuid, slots: usableAllocs[nextUUID.uuid].FreeSlots})
				}

				// We have enough slots now, stop looking and go to the next largest starting point
				if slotsFound >= maxGPU {
					return
				}
			}
		}()

		// We have a combination that meets or exceeds our needs
		if slotsFound >= maxGPU {
			cmd.waste = int(slotsFound - maxGPU)
			combinations = append(combinations, cmd)
		}
	}

	if len(combinations) == 0 {
		kvDetails = append(kvDetails, "stack", stack.Trace().TrimRuntime())
		return nil, kv.NewError("insufficient GPU").With(kvDetails...)
	}

	// Sort the combinations by waste, get the least waste
	//
	sort.Slice(combinations, func(i, j int) bool { return combinations[i].waste < combinations[j].waste })

	// Get all of the combinations that have the least and same waste in slots
	minWaste := combinations[0].waste
	for i, comb := range combinations {
		if minWaste != comb.waste {
			combinations = combinations[:i]
			break
		}
	}

	// Sort what is left over by the number of impacted cards
	sort.Slice(combinations, func(i, j int) bool { return len(combinations[i].cards) < len(combinations[j].cards) })
	kvDetails = append(kvDetails, []interface{}{"combinations", combinations}...)

	// OK Now we simply take the first option if one was found
	matched := combinations[0]

	if len(matched.cards) == 0 {
		kvDetails = append(kvDetails, "stack", stack.Trace().TrimRuntime())
		return nil, kv.NewError("insufficient partitioned GPUs").With(kvDetails...)
	}

	// Got as far as knowing the allocation will work so check for the live flag
	if !live {
		return nil, nil
	}

	// Go through the chosen combination of cards and do the allocations
	//
	for _, found := range matched.cards {
		slots := maxGPU
		if slots > allocator.Allocs[found.uuid].FreeSlots {
			slots = allocator.Allocs[found.uuid].FreeSlots
		}

		if maxGPUMem == 0 {
			// If the user does not know take it all, burn it to the ground
			slots = allocator.Allocs[found.uuid].FreeSlots
			maxGPUMem = allocator.Allocs[found.uuid].FreeMem
		}
		allocator.Allocs[found.uuid].FreeSlots -= slots
		allocator.Allocs[found.uuid].FreeMem -= maxGPUMem

		tracking := xid.New().String()
		alloc = append(alloc, &GPUAllocated{
			tracking: tracking,
			uuid:     found.uuid,
			slots:    slots,
			mem:      maxGPUMem,
			Env:      map[string]string{"CUDA_VISIBLE_DEVICES": found.uuid},
		})

		allocator.Allocs[found.uuid].Tracking[tracking] = struct{}{}
	}

	return alloc, nil
}

func (allocator *gpuTracker) ReturnGPU(alloc *GPUAllocated) (err kv.Error) {

	if alloc.slots == 0 {
		return nil
	}

	allocator.Lock()
	defer allocator.Unlock()

	// Make sure that the allocation is still valid
	if _, isPresent := allocator.Allocs[alloc.uuid]; !isPresent {
		return kv.NewError("cuda device no longer in service").With("device", alloc.uuid).With("stack", stack.Trace().TrimRuntime())
	}

	if _, isPresent := allocator.Allocs[alloc.uuid].Tracking[alloc.tracking]; !isPresent {
		return kv.NewError("invalid allocation").With("alloc_id", alloc.tracking).With("stack", stack.Trace().TrimRuntime())
	}

	delete(allocator.Allocs[alloc.uuid].Tracking, alloc.tracking)

	// If valid pass back the resources that were consumed
	allocator.Allocs[alloc.uuid].FreeSlots += alloc.slots
	allocator.Allocs[alloc.uuid].FreeMem += alloc.mem

	return nil
}

// ReturnGPU releases the GPU allocation passed in.  It will validate some of the allocation
// details but is an honors system.
//
func ReturnGPU(alloc *GPUAllocated) (err kv.Error) {
	return gpuAllocs.ReturnGPU(alloc)
}
