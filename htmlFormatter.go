package main

import (
	"bytes"
	"fmt"
	"html"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	red    = "red"
	yellow = "yellow"
	blue   = "blue"
	gray   = "grey"
)
const defaultTimestampFormat = time.RFC3339

var baseTimestamp time.Time

func init() {
	baseTimestamp = time.Now()
}

// HTMLFormatter formats logs into text
type HTMLFormatter struct {
	// Force disabling colors.
	DisableColors bool

	// Force quoting of all values
	ForceQuote bool

	// Disable timestamp logging. useful when output is redirected to logging
	// system that already adds timestamps.
	DisableTimestamp bool

	// Enable logging the full timestamp when a TTY is attached instead of just
	// the time passed since beginning of execution.
	FullTimestamp bool

	// TimestampFormat to use for display when a full timestamp is printed
	TimestampFormat string

	// The fields are sorted by default for a consistent output. For applications
	// that log extremely frequently and don't use the JSON formatter this may not
	// be desired.
	DisableSorting bool

	// The keys sorting function, when uninitialized it uses sort.Strings.
	SortingFunc func([]string)

	// QuoteEmptyFields will wrap empty fields in quotes if true
	QuoteEmptyFields bool

	// Whether the logger's out is to a terminal
	IsTerminal bool

	// FieldMap allows users to customize the names of keys for default fields.
	// As an example:
	// formatter := &HTMLFormatter{
	//     FieldMap: FieldMap{
	//         FieldKeyTime:  "@timestamp",
	//         FieldKeyLevel: "@level",
	//         FieldKeyMsg:   "@message"}}
	FieldMap log.FieldMap

	// CallerPrettyfier can be set by the user to modify the content
	// of the function and file keys in the data when ReportCaller is
	// activated. If any of the returned value is the empty string the
	// corresponding key will be removed from fields.
	CallerPrettyfier func(*runtime.Frame) (function string, file string)

	terminalInitOnce sync.Once
}

func (f *HTMLFormatter) init(entry *log.Entry) {
	fmt.Fprint(entry.Buffer, "<table>")
}

func (f *HTMLFormatter) isColored() bool {
	return !f.DisableColors
}

// Format renders a single log entry
func (f *HTMLFormatter) Format(entry *log.Entry) ([]byte, error) {
	data := make(log.Fields)
	for k, v := range entry.Data {
		data[k] = v
	}
	// prefixFieldClashes(data, f.FieldMap, entry.HasCaller())
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}

	var funcVal, fileVal string

	fixedKeys := make([]string, 0, 4+len(data))
	if !f.DisableTimestamp {
		fixedKeys = append(fixedKeys, log.FieldKeyTime)
	}
	fixedKeys = append(fixedKeys, log.FieldKeyLevel)
	if entry.Message != "" {
		fixedKeys = append(fixedKeys, log.FieldKeyMsg)
	}
	// if entry.err != "" {
	//     fixedKeys = append(fixedKeys, log.FieldKeyLogrusError)
	// }
	if entry.HasCaller() {
		if f.CallerPrettyfier != nil {
			funcVal, fileVal = f.CallerPrettyfier(entry.Caller)
		} else {
			funcVal = entry.Caller.Function
			fileVal = fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line)
		}

		if funcVal != "" {
			fixedKeys = append(fixedKeys, log.FieldKeyFunc)
		}
		if fileVal != "" {
			fixedKeys = append(fixedKeys, log.FieldKeyFile)
		}
	}

	if !f.DisableSorting {
		if f.SortingFunc == nil {
			sort.Strings(keys)
			fixedKeys = append(fixedKeys, keys...)
		} else {
			if !f.isColored() {
				fixedKeys = append(fixedKeys, keys...)
				f.SortingFunc(fixedKeys)
			} else {
				f.SortingFunc(keys)
			}
		}
	} else {
		fixedKeys = append(fixedKeys, keys...)
	}

	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}

	f.terminalInitOnce.Do(func() { f.init(entry) })

	timestampFormat := f.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = defaultTimestampFormat
	}

	if f.isColored() {
		f.printColored(b, entry, keys, data, timestampFormat)
	} else {

		for _, key := range fixedKeys {
			var value interface{}
			switch {
			case key == (log.FieldKeyTime):
				value = entry.Time.Format(timestampFormat)
			case key == (log.FieldKeyLevel):
				value = entry.Level.String()
			case key == (log.FieldKeyMsg):
				value = entry.Message
				// case key == (log.FieldKeyLogrusError):
				// value = entry.err
			case key == (log.FieldKeyFunc) && entry.HasCaller():
				value = funcVal
			case key == (log.FieldKeyFile) && entry.HasCaller():
				value = fileVal
			default:
				value = data[key]
			}
			f.appendKeyValue(b, key, value)
		}
	}

	b.WriteByte('\n')
	return b.Bytes(), nil
}

func (f *HTMLFormatter) printColored(b *bytes.Buffer, entry *log.Entry, keys []string, data log.Fields, timestampFormat string) {
	var levelColor string
	switch entry.Level {
	case log.DebugLevel, log.TraceLevel:
		levelColor = gray
	case log.WarnLevel:
		levelColor = yellow
	case log.ErrorLevel, log.FatalLevel, log.PanicLevel:
		levelColor = red
	default:
		levelColor = blue
	}

	levelText := strings.ToUpper(entry.Level.String())

	// Remove a single newline if it already exists in the message to keep
	// the behavior of logrus text_formatter the same as the stdlib log package
	entry.Message = strings.TrimSuffix(entry.Message, "\n")

	caller := ""
	if entry.HasCaller() {
		funcVal := fmt.Sprintf("%s()", entry.Caller.Function)
		fileVal := fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line)

		if f.CallerPrettyfier != nil {
			funcVal, fileVal = f.CallerPrettyfier(entry.Caller)
		}

		if fileVal == "" {
			caller = funcVal
		} else if funcVal == "" {
			caller = fileVal
		} else {
			caller = fileVal + " " + funcVal
		}
	}
	fmt.Fprint(b, "<tr>")
	if f.DisableTimestamp {
		fmt.Fprintf(b, "<td>%s</td><td>%s</td><td>%s</td>", levelText, caller, entry.Message)
	} else if !f.FullTimestamp {
		fmt.Fprintf(b, "<td>%s</td><td>%d</td><td>%s</td><td>%s</td>", levelText, int(entry.Time.Sub(baseTimestamp)/time.Second), caller, html.EscapeString(entry.Message))
	} else {
		if entry.Level == log.DebugLevel || entry.Level == log.TraceLevel {
			fmt.Fprintf(b, "<td style=\"color:%s\">%s</td><td>%s</td><td>%s</td><td><pre style=\"padding: 0 5px 0 5px;margin-bottom: 0;color:%s\">%s</pre></td>", levelColor, levelText, entry.Time.Format(timestampFormat), caller, levelColor, html.EscapeString(entry.Message))
		} else {
			fmt.Fprintf(b, "<td style=\"color:%s\">%s</td><td>%s</td><td>%s</td><td><pre style=\"padding: 0 5px 0 5px;margin-bottom: 0;\">%s</pre></td>", levelColor, levelText, entry.Time.Format(timestampFormat), caller, html.EscapeString(entry.Message))
		}
	}
	for _, k := range keys {
		v := data[k]
		if v == nil {
			continue
		}
		vs := f.getValueString(v)
		if len(vs) == 0 {
			continue
		}
		fmt.Fprintf(b, "<td><span style=\"color:%s\">%s</span>=%s</td>", levelColor, k, vs)
	}
	fmt.Fprint(b, "</tr>")
}

func (f *HTMLFormatter) needsQuoting(text string) bool {
	if f.ForceQuote {
		return true
	}
	if f.QuoteEmptyFields && len(text) == 0 {
		return true
	}
	for _, ch := range text {
		if !((ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '.' || ch == '_' || ch == '/' || ch == '@' || ch == '^' || ch == '+') {
			return true
		}
	}
	return false
}

func (f *HTMLFormatter) appendKeyValue(b *bytes.Buffer, key string, value interface{}) {
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	b.WriteString(key)
	b.WriteByte('=')
	vs := f.getValueString(value)
	b.WriteString(vs)
}

func (f *HTMLFormatter) getValueString(value interface{}) string {
	stringVal, ok := value.(string)
	if !ok {
		stringVal = fmt.Sprint(value)
	}
	stringVal = html.EscapeString(stringVal)

	if !f.needsQuoting(stringVal) {
		return stringVal
	}
	return fmt.Sprintf("%q", stringVal)
}
