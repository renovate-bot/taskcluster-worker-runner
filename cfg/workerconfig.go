package cfg

import (
	"encoding/json"
	"fmt"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

// The configuration that will be passed directly to the worker.  At
// the top level, this is a JSON object, but it can contain arbitrary
// other JSON types within it.
//
// Treat this as a read-only data structure, replacing it as necessary
// using the methods provided below.
type WorkerConfig struct {
	data map[string]interface{}
}

// Normalize a JSON value, using the same types regardless of source
//
// Specifically, maps should be map[string]value, and numbers should
// be float64s
func normalize(value interface{}) interface{} {
	strmap, ok := value.(map[string]interface{})
	if ok {
		res := make(map[string]interface{})
		for key, value := range strmap {
			res[key] = normalize(value)
		}
		return res
	}

	ifmap, ok := value.(map[interface{}]interface{})
	if ok {
		res := make(map[string]interface{})
		for key, value := range ifmap {
			res[key.(string)] = normalize(value)
		}
		return res
	}

	num, ok := value.(int)
	if ok {
		return float64(num)
	}

	return value
}

func (wc *WorkerConfig) UnmarshalYAML(node *yaml.Node) error {
	var res map[string]interface{}
	err := node.Decode(&res)
	if err != nil {
		return err
	}
	wc.data = normalize(res).(map[string]interface{})
	return nil
}

func (wc *WorkerConfig) UnmarshalJSON(b []byte) error {
	var res map[string]interface{}
	err := json.Unmarshal(b, &res)
	if err != nil {
		return err
	}
	wc.data = normalize(res).(map[string]interface{})
	return nil
}

func merge(v1, v2 interface{}) interface{} {
	// if both are maps, merge them
	map1, map1ok := v1.(map[string]interface{})
	map2, map2ok := v2.(map[string]interface{})
	if map1ok && map2ok {
		res := make(map[string]interface{})
		for key, value := range map1 {
			res[key] = value
		}
		for key, value := range map2 {
			existing, ok := res[key]
			if ok {
				res[key] = merge(existing, value)
			} else {
				res[key] = value
			}
		}

		return res
	}

	// if both are arrays, concatenate them
	arr1, arr1ok := v1.([]interface{})
	arr2, arr2ok := v2.([]interface{})
	if arr1ok && arr2ok {
		res := make([]interface{}, 0, len(arr1)+len(arr2))
		for _, value := range arr1 {
			res = append(res, value)
		}
		for _, value := range arr2 {
			res = append(res, value)
		}

		return res
	}

	// otherwise, just use the second value, overriding the first
	return v2
}

// Merge two WorkerConfig objects, preferring values from the second
// object where both are provided.  Where both objects have an object
// as a value, those objects are merged recursively.  Where both objects
// have an array as a value, those arrays are concatenated.
//
// This returns a new WorkerConfig without modifying either input.
func (wc *WorkerConfig) Merge(other *WorkerConfig) *WorkerConfig {
	return &WorkerConfig{
		data: merge(wc.data, other.data).(map[string]interface{}),
	}
}

func set(key []string, i int, config interface{}, value interface{}) (interface{}, error) {
	if i == len(key) {
		return value, nil
	}

	configmap, ok := config.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s is not an object in existing config", strings.Join(key[:i], "."))
	}

	clone := make(map[string]interface{})
	for k, v := range configmap {
		clone[k] = v
	}

	k := key[i]
	v, ok := clone[k]
	if !ok {
		v = make(map[string]interface{})
		clone[k] = v
	}

	var err error
	clone[k], err = set(key, i+1, v, value)
	if err != nil {
		return nil, err
	}

	return clone, nil
}

// Set a value at the given dotted path.
//
// This returns a new WorkerConfig containing the updated value.
func (wc *WorkerConfig) Set(key string, value interface{}) (*WorkerConfig, error) {
	if key == "" {
		return nil, fmt.Errorf("Must specify a nonempty key")
	}

	splitkey := strings.Split(key, ".")
	data, err := set(splitkey, 0, wc.data, value)
	if err != nil {
		return nil, err
	}
	return &WorkerConfig{
		data: data.(map[string]interface{}),
	}, nil
}