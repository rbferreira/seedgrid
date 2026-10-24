package rtorrent

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// The XML-RPC codec rTorrent needs, and no more (ADR-045 §2): rTorrent speaks XML-RPC over SCGI or
// through a web server, and a library would be the Agent's first dependency for a format this small.
//
// Values map to Go as: string ↔ string, int/int64 ↔ i4/i8, bool ↔ boolean, []any ↔ array,
// map[string]any ↔ struct. Decoding also gives float64 (double) and []byte (base64). A value
// without a type is a string, as the spec says.

// Fault is an XML-RPC fault: rTorrent refused the call (unknown command, unknown hash…).
type Fault struct {
	Code    int
	Message string
}

func (f *Fault) Error() string { return fmt.Sprintf("rtorrent: fault %d: %s", f.Code, f.Message) }

// encodeCall writes a methodCall.
func encodeCall(method string, params ...any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0"?><methodCall><methodName>`)
	if err := xml.EscapeText(&b, []byte(method)); err != nil {
		return nil, err
	}
	b.WriteString(`</methodName><params>`)
	for _, p := range params {
		b.WriteString(`<param>`)
		if err := encodeValue(&b, p); err != nil {
			return nil, err
		}
		b.WriteString(`</param>`)
	}
	b.WriteString(`</params></methodCall>`)
	return b.Bytes(), nil
}

func encodeValue(b *bytes.Buffer, v any) error {
	b.WriteString(`<value>`)
	switch x := v.(type) {
	case string:
		b.WriteString(`<string>`)
		if err := xml.EscapeText(b, []byte(x)); err != nil {
			return err
		}
		b.WriteString(`</string>`)
	case int:
		writeInt(b, int64(x))
	case int64:
		writeInt(b, x)
	case bool:
		if x {
			b.WriteString(`<boolean>1</boolean>`)
		} else {
			b.WriteString(`<boolean>0</boolean>`)
		}
	case []byte:
		b.WriteString(`<base64>` + base64.StdEncoding.EncodeToString(x) + `</base64>`)
	case []string:
		b.WriteString(`<array><data>`)
		for _, e := range x {
			if err := encodeValue(b, e); err != nil {
				return err
			}
		}
		b.WriteString(`</data></array>`)
	case []any:
		b.WriteString(`<array><data>`)
		for _, e := range x {
			if err := encodeValue(b, e); err != nil {
				return err
			}
		}
		b.WriteString(`</data></array>`)
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(`<struct>`)
		for _, k := range keys {
			b.WriteString(`<member><name>`)
			if err := xml.EscapeText(b, []byte(k)); err != nil {
				return err
			}
			b.WriteString(`</name>`)
			if err := encodeValue(b, x[k]); err != nil {
				return err
			}
			b.WriteString(`</member>`)
		}
		b.WriteString(`</struct>`)
	default:
		return fmt.Errorf("rtorrent: cannot encode %T", v)
	}
	b.WriteString(`</value>`)
	return nil
}

// writeInt uses i4 when the value fits, i8 otherwise (rTorrent reads both).
func writeInt(b *bytes.Buffer, n int64) {
	tag := "i4"
	if n > 1<<31-1 || n < -1<<31 {
		tag = "i8"
	}
	b.WriteString("<" + tag + ">" + strconv.FormatInt(n, 10) + "</" + tag + ">")
}

// decodeResponse reads a methodResponse: its single value, or the Fault.
func decodeResponse(r io.Reader) (any, error) {
	d := xml.NewDecoder(r) //nolint:gosec // G709: decoded into plain values only (no types chosen by the input), from the local Engine, size-bounded by maxResponse
	if err := expectPath(d, "methodResponse"); err != nil {
		return nil, err
	}
	se, err := nextStart(d)
	if err != nil {
		return nil, err
	}
	switch se.Name.Local {
	case "params":
		if err := expectPath(d, "param", "value"); err != nil {
			return nil, err
		}
		return decodeValue(d)
	case "fault":
		if err := expectPath(d, "value"); err != nil {
			return nil, err
		}
		v, err := decodeValue(d)
		if err != nil {
			return nil, err
		}
		m, _ := v.(map[string]any)
		code, _ := m["faultCode"].(int64)
		msg, _ := m["faultString"].(string)
		return nil, &Fault{Code: int(code), Message: msg}
	default:
		return nil, fmt.Errorf("rtorrent: unexpected <%s> in methodResponse", se.Name.Local)
	}
}

// decodeValue reads the content of a <value> whose start tag was consumed, through its end tag.
func decodeValue(d *xml.Decoder) (any, error) {
	var text strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.EndElement: // </value> with no type: a string
			return text.String(), nil
		case xml.StartElement:
			v, err := decodeTyped(d, t.Name.Local)
			if err != nil {
				return nil, err
			}
			if err := skipToEnd(d); err != nil { // </value>
				return nil, err
			}
			return v, nil
		}
	}
}

func decodeTyped(d *xml.Decoder, typ string) (any, error) {
	switch typ {
	case "array":
		if err := expectPath(d, "data"); err != nil {
			return nil, err
		}
		out := []any{}
		for {
			se, err := nextStart(d)
			if errors.Is(err, errEnd) { // </data>
				return out, skipToEnd(d) // </array>
			}
			if err != nil {
				return nil, err
			}
			if se.Name.Local != "value" {
				return nil, fmt.Errorf("rtorrent: unexpected <%s> in array", se.Name.Local)
			}
			v, err := decodeValue(d)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	case "struct":
		out := map[string]any{}
		for {
			se, err := nextStart(d)
			if errors.Is(err, errEnd) { // </struct>
				return out, nil
			}
			if err != nil {
				return nil, err
			}
			if se.Name.Local != "member" {
				return nil, fmt.Errorf("rtorrent: unexpected <%s> in struct", se.Name.Local)
			}
			var name string
			var val any
			for {
				me, err := nextStart(d)
				if errors.Is(err, errEnd) { // </member>
					break
				}
				if err != nil {
					return nil, err
				}
				switch me.Name.Local {
				case "name":
					if name, err = text(d); err != nil {
						return nil, err
					}
				case "value":
					if val, err = decodeValue(d); err != nil {
						return nil, err
					}
				default:
					return nil, fmt.Errorf("rtorrent: unexpected <%s> in member", me.Name.Local)
				}
			}
			out[name] = val
		}
	}
	s, err := text(d)
	if err != nil {
		return nil, err
	}
	switch typ {
	case "string":
		return s, nil
	case "i4", "i8", "int":
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("rtorrent: bad <%s>: %w", typ, err)
		}
		return n, nil
	case "boolean":
		return strings.TrimSpace(s) == "1", nil
	case "double":
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return nil, fmt.Errorf("rtorrent: bad <double>: %w", err)
		}
		return f, nil
	case "base64":
		return base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	default:
		return nil, fmt.Errorf("rtorrent: unknown type <%s>", typ)
	}
}

// errEnd: nextStart met an end tag (the enclosing element closed) instead of a start tag.
var errEnd = errors.New("end element")

// nextStart returns the next start tag, skipping whitespace; errEnd at an end tag.
func nextStart(d *xml.Decoder) (xml.StartElement, error) {
	for {
		tok, err := d.Token()
		if err != nil {
			return xml.StartElement{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			return t, nil
		case xml.EndElement:
			return xml.StartElement{}, errEnd
		}
	}
}

// expectPath consumes the start tags of names, in order.
func expectPath(d *xml.Decoder, names ...string) error {
	for _, n := range names {
		se, err := nextStart(d)
		if err != nil {
			return fmt.Errorf("rtorrent: expected <%s>: %w", n, err)
		}
		if se.Name.Local != n {
			return fmt.Errorf("rtorrent: expected <%s>, got <%s>", n, se.Name.Local)
		}
	}
	return nil
}

// text reads an element's character data through its end tag.
func text(d *xml.Decoder) (string, error) {
	var b strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			return b.String(), nil
		case xml.StartElement:
			return "", fmt.Errorf("rtorrent: unexpected <%s> in a scalar", t.Name.Local)
		}
	}
}

// skipToEnd consumes tokens through the next end tag at this level.
func skipToEnd(d *xml.Decoder) error {
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.EndElement:
			return nil
		case xml.StartElement:
			return errors.New("rtorrent: unexpected element")
		}
	}
}
