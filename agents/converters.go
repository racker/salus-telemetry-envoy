/*
 * Copyright 2019 Rackspace US, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agents

import (
	"bytes"
	"encoding/json"
	"github.com/BurntSushi/toml"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"strings"
	"time"
)

type Conversion int

const (
	ConversionNone = iota
	ConversionJsonToTelegrafToml
)

func ConvertJsonToTelegrafToml(configJson string, extraLabels map[string]string, interval int64) ([]byte, error) {

	jsonDecoder := json.NewDecoder(strings.NewReader(configJson))
	jsonDecoder.UseNumber()
	var flatMap map[string]interface{}

	err := jsonDecoder.Decode(&flatMap)
	if err != nil {
		return nil, errors.Wrap(err, "unable to decode raw config json")
	}

	typeVal, typeExists := flatMap["type"]
	if !typeExists {
		return nil, errors.New("missing required 'type' discriminator")
	}
	// remove type from map to avoid it ending up in TOML body
	delete(flatMap, "type")

	pluginName, typeValid := typeVal.(string)
	if !typeValid {
		return nil, errors.New("'type' needs to be a string")
	}

	if pluginName == "" {
		return nil, errors.New("'type' needs to be non-empty")
	}

	// convert to inputs->{plugin_name}->[]{plugin config}
	mapOfLists := make(map[string]map[string][]map[string]interface{}, 1)
	inputPlugins := make(map[string][]map[string]interface{}, len(flatMap))
	mapOfLists["inputs"] = inputPlugins

	finalPluginConfig := normalizeKeysAndValues(flatMap)
	if extraLabels != nil && len(extraLabels) > 0 {
		finalPluginConfig["tags"] = extraLabels
	}

	if interval != 0 {
		finalPluginConfig["interval"] = (time.Duration(interval) * time.Second).String()
	}

	inputPlugins[pluginName] = append(inputPlugins[pluginName], finalPluginConfig)

	var tomlBuffer bytes.Buffer
	tomlEncoder := toml.NewEncoder(&tomlBuffer)
	err = tomlEncoder.Encode(mapOfLists)
	if err != nil {
		return nil, errors.Wrap(err, "unable to encode config as toml")
	}

	return tomlBuffer.Bytes(), nil
}

// normalizeKeysAndValues converts the key names to lower_snake_case and adjusts numerical values
// to be int or float specific.
func normalizeKeysAndValues(config map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(config))

	for k, v := range config {
		if asNumber, ok := v.(json.Number); ok {
			if asInt, err := asNumber.Int64(); err == nil {
				v = asInt
			} else if asFloat, err := asNumber.Float64(); err == nil {
				v = asFloat
			} else {
				// fallback to string
				v = asNumber.String()
			}
		}
		normalized[strcase.ToSnake(k)] = v
	}

	return normalized
}
