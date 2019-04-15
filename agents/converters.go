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
)

type Conversion int

const (
	ConversionNone = iota
	ConversionJsonToTelegrafToml
)

func ConvertJsonToTelegrafToml(configJson string, extraLabels map[string]string) ([]byte, error) {

	jsonDecoder := json.NewDecoder(strings.NewReader(configJson))
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

	finalPluginConfig := normalizeKeys(flatMap)
	if extraLabels != nil && len(extraLabels) > 0 {
		finalPluginConfig["tags"] = extraLabels
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

// normalizeKeys converts the key names to lower_snake_case
func normalizeKeys(config map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(config))

	for k, v := range config {
		normalized[strcase.ToSnake(k)] = v
	}

	return normalized
}
