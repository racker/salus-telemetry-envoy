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
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/ebnf"
	"github.com/racker/telemetry-envoy/telemetry_edge"
)

var (
	influxLineLexer = lexer.Must(ebnf.New(`
		Int = [ "-" | "+" ] digit { digit } .
		Float = [ "-" | "+" ] ("." | digit) {"." | digit} [ ("e"|"E") "+" { digit } ] .

		alpha = "a"…"z" | "A"…"Z" .
		digit = "0"…"9" .
		any = "\u0000"…"\uffff" .
`))
)

type InfluxLine struct {
}

type FieldValue struct {
	Int   *int64   `  @Int "i"`
	Float *float64 `| @Float`
}

func ParseInfluxLineProtocolMetrics(content []byte) (*telemetry_edge.Metric, error) {
	return nil, nil
}
