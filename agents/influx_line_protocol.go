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
	"strconv"
	"strings"
)

/*
This parser parses influx line protocol, https://docs.influxdata.com/influxdb/v1.7/write_protocols/line_protocol_reference/

<measurement>[,<tag_key>=<tag_value>[,<tag_key>=<tag_value>]] <field_key>=<field_value>[,<field_key>=<field_value>] [<timestamp>]

Tokens preceded with an '@' capture the value of the token into the corresponding field, e.g:
this grammar:

type InfluxTagSet struct {
	Key   string `@String Equals`
	Value string `(@String | @Float | @Int)`
}

when given this input:

"fstype=devfs",

will set 'Key' to 'fstype' and 'Value' to 'devfs'.

'@@' captures the entire struct, and '@@*' captures the entire array of structs
More details about the grammar definitions here: // https://github.com/alecthomas/participle
*/

type InfluxLines struct {
	Lines []*InfluxLine `@@ (Newline @@)* Newline?`
}

type InfluxLine struct {
	MetricName string            `TestOutputPrefix? @String`
	TagSet     []*InfluxTagSet   `( "," @@ )*`
	FieldSet   []*InfluxFieldSet `Space @@ ( "," @@ )*`
	// It's actually a long, but Float is the most appropriate token since Int has the trailing "i" thing
	// Participle will take care of coercing the number into a uint64
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

	results := make([]*telemetry_edge.NameTagValueMetric, 0, len(result.Lines))
	for _, line := range result.Lines {
		metric := &telemetry_edge.NameTagValueMetric{
			Name: line.MetricName,
			// convert nanosecond influx timestamp to milliseconds
			Timestamp: int64(line.Timestamp / 1000000),
		}

		metric.Tags = make(map[string]string)
		for _, tag := range line.TagSet {
			metric.Tags[tag.Key] = tag.Value
		}

		metric.Fvalues = make(map[string]float64)
		metric.Svalues = make(map[string]string)
		for _, field := range line.FieldSet {
			if field.Value.String != nil {
				metric.Svalues[field.Key] = *field.Value.String
			} else if field.Value.Boolean != nil {
				metric.Svalues[field.Key] = strconv.FormatBool(*field.Value.Boolean)
			} else if field.Value.Float != nil {
				metric.Fvalues[field.Key] = *field.Value.Float
			} else if field.Value.Int != nil {
				// need to match schema behavior of ingest/telegraf_json where JSON unmarshalling
				// cannot differentiate between float64 and int64
				metric.Fvalues[field.Key] = float64(*field.Value.Int)
			}
		}

		results = append(results, metric)
	}

	return results, nil
}
