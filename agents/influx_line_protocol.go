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
	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/ebnf"
	"github.com/racker/telemetry-envoy/telemetry_edge"
	"strings"
)

type InfluxLines struct {
	Lines []*InfluxLine `@@ (Newline @@)* Newline?`
}

type InfluxLine struct {
	MetricName string            `TestOutputPrefix? @String`
	TagSet     []*InfluxTagSet   `( "," @@ )*`
	FieldSet   []*InfluxFieldSet `Space @@ ( "," @@ )*`
	// it's not a Float, but that's actually the most appropriate token since Int has the trailing "i" thing
	Timestamp uint64 `Space @Float`
}

type InfluxTagSet struct {
	Key   string `@String Equals`
	Value string `(@String | @Float | @Int)`
}

type InfluxFieldSet struct {
	Key   string            `@String Equals`
	Value *InfluxFieldValue `@@`
}

type InfluxFieldValue struct {
	// integers have to be lexed into a string since Influx line protocol requires "i" suffix
	Int     *uint64  `  @Int`
	Float   *float64 `| @Float`
	Boolean *bool    `| ( @("t"|"T"|"True"|"TRUE") | ("f"|"F"|"False"|"FALSE") )`
	String  *string  `| ( @String | @QuotedString )`
}

var (
	influxLineLexer = lexer.Must(ebnf.New(`
		Int = [ "-" | "+" ] digit {digit} "i" .
		Float = [ "-" | "+" ] ("." | digit) {"." | digit} [ ("e"|"E") "+" { digit } ] .
		Comma = "," .
		Space = " " .
		Equals = "=" .
		Newline = "\r\n" | "\r" | "\n" .
		TestOutputPrefix = "> " .
		QuotedString = "\"" { "\u0000"…"\uffff"-"\""-"\\" | "\\" any } "\"" .
		String = { "\u0000"…"\uffff"-"\\"-","-" "-"=" | "\\" any } .

		alpha = "a"…"z" | "A"…"Z" .
		digit = "0"…"9" .
		any = "\u0000"…"\uffff" .
`))
	influxLineParser = participle.MustBuild(&InfluxLines{},
		participle.Lexer(influxLineLexer),
		participle.Map(fixInfluxLineInt, "Int"),
		participle.Unquote("QuotedString"),
	)
)

func fixInfluxLineInt(t lexer.Token) (lexer.Token, error) {
	if strings.HasSuffix(t.Value, "i") {
		t.Value = t.Value[:len(t.Value)-1]
	}
	return t, nil
}

func ParseInfluxLineProtocolMetrics(content []byte) ([]*telemetry_edge.NameTagValueMetric, error) {
	result := &InfluxLines{}
	err := influxLineParser.ParseBytes(content, result)
	if err != nil {
		return nil, err
	}

	type GroupingKey struct {
		MetricName string
		Timestamp  uint64
	}

	groupedMetrics := make(map[GroupingKey]*telemetry_edge.NameTagValueMetric)

	for _, line := range result.Lines {
		key := GroupingKey{MetricName: line.MetricName, Timestamp: line.Timestamp}
		metric := groupedMetrics[key]
		if metric == nil {
			metric = &telemetry_edge.NameTagValueMetric{
				Name: line.MetricName,
				// convert nanosecond influx timestamp to milliseconds
				Timestamp: int64(line.Timestamp / 1000000),
			}
		}

		// TODO convert tagset and fieldset

		groupedMetrics[key] = metric
	}

	results := make([]*telemetry_edge.NameTagValueMetric, 0, len(groupedMetrics))
	for _, metric := range groupedMetrics {
		results = append(results, metric)
	}
	return results, nil
}