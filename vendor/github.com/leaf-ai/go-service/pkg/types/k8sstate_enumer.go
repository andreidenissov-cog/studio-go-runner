// Code generated by "enumer -type K8sState -trimprefix K8s"; DO NOT EDIT.

package types

import (
	"fmt"
)

const _K8sStateName = "UnknownRunningDrainAndTerminateDrainAndSuspend"

var _K8sStateIndex = [...]uint8{0, 7, 14, 31, 46}

func (i K8sState) String() string {
	if i < 0 || i >= K8sState(len(_K8sStateIndex)-1) {
		return fmt.Sprintf("K8sState(%d)", i)
	}
	return _K8sStateName[_K8sStateIndex[i]:_K8sStateIndex[i+1]]
}

var _K8sStateValues = []K8sState{0, 1, 2, 3}

var _K8sStateNameToValueMap = map[string]K8sState{
	_K8sStateName[0:7]:   0,
	_K8sStateName[7:14]:  1,
	_K8sStateName[14:31]: 2,
	_K8sStateName[31:46]: 3,
}

// K8sStateString retrieves an enum value from the enum constants string name.
// Throws an error if the param is not part of the enum.
func K8sStateString(s string) (K8sState, error) {
	if val, ok := _K8sStateNameToValueMap[s]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("%s does not belong to K8sState values", s)
}

// K8sStateValues returns all values of the enum
func K8sStateValues() []K8sState {
	return _K8sStateValues
}

// IsAK8sState returns "true" if the value is listed in the enum definition. "false" otherwise
func (i K8sState) IsAK8sState() bool {
	for _, v := range _K8sStateValues {
		if i == v {
			return true
		}
	}
	return false
}