// Package ipp is the standards IPP driver (RFC 8010 encoding, RFC 8011
// semantics, PWG 5100.13 for printer-supply / printer-input-tray strings).
// This file is a small self-contained IPP message codec.
package ipp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Value tags (RFC 8010 §3.5.2).
const (
	TagOperation   byte = 0x01
	TagJob         byte = 0x02
	TagEnd         byte = 0x03
	TagPrinter     byte = 0x04
	TagUnsupported byte = 0x05

	TagUnsupportedValue byte = 0x10
	TagDefault          byte = 0x11
	TagUnknown          byte = 0x12
	TagNoValue          byte = 0x13

	TagInteger       byte = 0x21
	TagBoolean       byte = 0x22
	TagEnum          byte = 0x23
	TagOctet         byte = 0x30
	TagDateTime      byte = 0x31
	TagResolution    byte = 0x32
	TagRange         byte = 0x33
	TagBegCollection byte = 0x34
	TagTextLang      byte = 0x35
	TagNameLang      byte = 0x36
	TagEndCollection byte = 0x37
	TagText          byte = 0x41
	TagName          byte = 0x42
	TagKeyword       byte = 0x44
	TagURI           byte = 0x45
	TagURIScheme     byte = 0x46
	TagCharset       byte = 0x47
	TagLanguage      byte = 0x48
	TagMime          byte = 0x49
	TagMemberName    byte = 0x4a
)

// OpGetPrinterAttributes is the operation id (RFC 8011 §4.2.5).
const OpGetPrinterAttributes uint16 = 0x000B

// OpSetPrinterAttributes is the operation id (RFC 3380 §4.1), the only IPP
// write the configuration feature issues.
const OpSetPrinterAttributes uint16 = 0x0013

// IPP status codes used by the driver (RFC 8011 Appendix B, RFC 3380 §8).
const (
	StatusOK                          uint16 = 0x0000
	StatusOKIgnoredOrSubstituted      uint16 = 0x0001
	StatusOKConflicting               uint16 = 0x0002
	StatusClientErrorNotPossible      uint16 = 0x0403
	StatusClientErrorAttrsNotSettable uint16 = 0x0413 // RFC 3380 §8.1
	StatusServerErrorOpNotSupported   uint16 = 0x0501
)

// Attribute is one IPP attribute with one or more values.
type Attribute struct {
	Tag    byte
	Name   string
	Values []any // int, bool, string, []byte, map[string]Attribute (collection)
}

// Message is a decoded IPP request or response.
type Message struct {
	Version    [2]byte
	StatusCode uint16 // operation id for requests
	RequestID  uint32
	Groups     []Group
}

// Group is one attribute group.
type Group struct {
	Tag        byte
	Attributes []Attribute
}

// Attr finds an attribute by name across all groups.
func (m *Message) Attr(name string) *Attribute {
	for gi := range m.Groups {
		for ai := range m.Groups[gi].Attributes {
			if m.Groups[gi].Attributes[ai].Name == name {
				return &m.Groups[gi].Attributes[ai]
			}
		}
	}
	return nil
}

// String returns the first value as text, or "".
func (m *Message) String(name string) string {
	a := m.Attr(name)
	if a == nil || len(a.Values) == 0 {
		return ""
	}
	switch v := a.Values[0].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case int:
		return fmt.Sprint(v)
	}
	return ""
}

// Strings returns all values as text.
func (m *Message) Strings(name string) []string {
	a := m.Attr(name)
	if a == nil {
		return nil
	}
	var out []string
	for _, v := range a.Values {
		switch x := v.(type) {
		case string:
			out = append(out, x)
		case []byte:
			out = append(out, string(x))
		case int:
			out = append(out, fmt.Sprint(x))
		}
	}
	return out
}

// Int returns the first integer/enum value.
func (m *Message) Int(name string) (int, bool) {
	a := m.Attr(name)
	if a == nil || len(a.Values) == 0 {
		return 0, false
	}
	n, ok := a.Values[0].(int)
	return n, ok
}

// Ints returns all integer values.
func (m *Message) Ints(name string) []int {
	a := m.Attr(name)
	if a == nil {
		return nil
	}
	var out []int
	for _, v := range a.Values {
		if n, ok := v.(int); ok {
			out = append(out, n)
		}
	}
	return out
}

// Encode serialises a message (RFC 8010 §3.1).
func Encode(m *Message) ([]byte, error) {
	var b bytes.Buffer
	b.Write(m.Version[:])
	binary.Write(&b, binary.BigEndian, m.StatusCode)
	binary.Write(&b, binary.BigEndian, m.RequestID)
	for _, g := range m.Groups {
		b.WriteByte(g.Tag)
		for _, a := range g.Attributes {
			if err := encodeAttr(&b, a); err != nil {
				return nil, err
			}
		}
	}
	b.WriteByte(TagEnd)
	return b.Bytes(), nil
}

func writeStr(b *bytes.Buffer, s string) {
	binary.Write(b, binary.BigEndian, uint16(len(s)))
	b.WriteString(s)
}

func encodeAttr(b *bytes.Buffer, a Attribute) error {
	if len(a.Values) == 0 {
		b.WriteByte(TagNoValue)
		writeStr(b, a.Name)
		binary.Write(b, binary.BigEndian, uint16(0))
		return nil
	}
	for i, v := range a.Values {
		name := a.Name
		if i > 0 {
			name = "" // additional value (RFC 8010 §3.1.5)
		}
		if coll, ok := v.(map[string]Attribute); ok {
			if err := writeCollection(b, name, coll); err != nil {
				return err
			}
			continue
		}
		b.WriteByte(a.Tag)
		writeStr(b, name)
		if err := writeValue(b, a.Tag, v); err != nil {
			return err
		}
	}
	return nil
}

// writeCollection encodes a collection value (RFC 8010 §3.1.6), recursing for nested collections.
func writeCollection(b *bytes.Buffer, name string, coll map[string]Attribute) error {
	b.WriteByte(TagBegCollection)
	writeStr(b, name)
	binary.Write(b, binary.BigEndian, uint16(0))
	for mname, ma := range coll {
		b.WriteByte(TagMemberName)
		writeStr(b, "")
		writeStr(b, mname)
		for _, mv := range ma.Values {
			if inner, ok := mv.(map[string]Attribute); ok {
				if err := writeCollection(b, "", inner); err != nil {
					return err
				}
				continue
			}
			b.WriteByte(ma.Tag)
			writeStr(b, "")
			if err := writeValue(b, ma.Tag, mv); err != nil {
				return err
			}
		}
	}
	b.WriteByte(TagEndCollection)
	writeStr(b, "")
	binary.Write(b, binary.BigEndian, uint16(0))
	return nil
}

func writeValue(b *bytes.Buffer, tag byte, v any) error {
	switch tag {
	case TagInteger, TagEnum:
		n, ok := v.(int)
		if !ok {
			return fmt.Errorf("ipp: %q expects int", attrTagName(tag))
		}
		binary.Write(b, binary.BigEndian, uint16(4))
		binary.Write(b, binary.BigEndian, int32(n))
	case TagBoolean:
		x, ok := v.(bool)
		if !ok {
			return errors.New("ipp: boolean expects bool")
		}
		binary.Write(b, binary.BigEndian, uint16(1))
		if x {
			b.WriteByte(1)
		} else {
			b.WriteByte(0)
		}
	case TagRange:
		r, ok := v.([2]int)
		if !ok {
			return errors.New("ipp: rangeOfInteger expects [2]int")
		}
		binary.Write(b, binary.BigEndian, uint16(8))
		binary.Write(b, binary.BigEndian, int32(r[0]))
		binary.Write(b, binary.BigEndian, int32(r[1]))
	case TagNoValue, TagUnknown, TagUnsupportedValue, TagDefault:
		binary.Write(b, binary.BigEndian, uint16(0))
	default:
		switch x := v.(type) {
		case string:
			writeStr(b, x)
		case []byte:
			binary.Write(b, binary.BigEndian, uint16(len(x)))
			b.Write(x)
		default:
			return fmt.Errorf("ipp: unsupported value %T for tag 0x%02x", v, tag)
		}
	}
	return nil
}

func attrTagName(tag byte) string {
	switch tag {
	case TagInteger:
		return "integer"
	case TagEnum:
		return "enum"
	}
	return fmt.Sprintf("0x%02x", tag)
}

// Decode parses a message.
func Decode(data []byte) (*Message, error) {
	r := bytes.NewReader(data)
	m := &Message{}
	if _, err := io.ReadFull(r, m.Version[:]); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.BigEndian, &m.StatusCode); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.BigEndian, &m.RequestID); err != nil {
		return nil, err
	}
	var cur *Group
	for {
		tag, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("ipp: truncated message")
		}
		if tag == TagEnd {
			return m, nil
		}
		if tag < 0x10 { // delimiter tag: new group
			m.Groups = append(m.Groups, Group{Tag: tag})
			cur = &m.Groups[len(m.Groups)-1]
			continue
		}
		name, err := readStr(r)
		if err != nil {
			return nil, err
		}
		val, err := readValue(r, tag)
		if err != nil {
			return nil, err
		}
		if cur == nil {
			m.Groups = append(m.Groups, Group{Tag: TagOperation})
			cur = &m.Groups[len(m.Groups)-1]
		}
		if tag == TagBegCollection {
			coll, err := readCollection(r)
			if err != nil {
				return nil, err
			}
			val = coll
		}
		if name == "" && len(cur.Attributes) > 0 {
			last := &cur.Attributes[len(cur.Attributes)-1]
			last.Values = append(last.Values, val)
			continue
		}
		cur.Attributes = append(cur.Attributes, Attribute{Tag: tag, Name: name, Values: []any{val}})
	}
}

func readStr(r *bytes.Reader) (string, error) {
	var n uint16
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return "", err
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func readValue(r *bytes.Reader, tag byte) (any, error) {
	var n uint16
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	switch tag {
	case TagInteger, TagEnum:
		if len(buf) != 4 {
			return nil, errors.New("ipp: bad integer length")
		}
		return int(int32(binary.BigEndian.Uint32(buf))), nil
	case TagBoolean:
		return len(buf) == 1 && buf[0] != 0, nil
	case TagRange:
		if len(buf) != 8 {
			return nil, errors.New("ipp: bad range length")
		}
		return [2]int{int(int32(binary.BigEndian.Uint32(buf[:4]))), int(int32(binary.BigEndian.Uint32(buf[4:])))}, nil
	case TagTextLang, TagNameLang:
		// language-length, language, value-length, value
		if len(buf) < 2 {
			return "", nil
		}
		ll := int(binary.BigEndian.Uint16(buf[:2]))
		if len(buf) < 2+ll+2 {
			return "", nil
		}
		vl := int(binary.BigEndian.Uint16(buf[2+ll : 4+ll]))
		if len(buf) < 4+ll+vl {
			return "", nil
		}
		return string(buf[4+ll : 4+ll+vl]), nil
	case TagOctet, TagDateTime, TagResolution:
		return buf, nil
	case TagBegCollection, TagEndCollection, TagMemberName:
		return buf, nil
	default:
		return string(buf), nil
	}
}

// readCollection consumes member attributes until the matching endCollection.
func readCollection(r *bytes.Reader) (map[string]Attribute, error) {
	coll := map[string]Attribute{}
	member := ""
	for {
		tag, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if _, err := readStr(r); err != nil { // name is always empty inside a collection
			return nil, err
		}
		switch tag {
		case TagEndCollection:
			if _, err := readValue(r, tag); err != nil {
				return nil, err
			}
			return coll, nil
		case TagMemberName:
			v, err := readValue(r, tag)
			if err != nil {
				return nil, err
			}
			member = string(v.([]byte))
		case TagBegCollection:
			if _, err := readValue(r, tag); err != nil {
				return nil, err
			}
			inner, err := readCollection(r)
			if err != nil {
				return nil, err
			}
			a := coll[member]
			a.Tag, a.Values = tag, append(a.Values, inner)
			coll[member] = a
		default:
			v, err := readValue(r, tag)
			if err != nil {
				return nil, err
			}
			a := coll[member]
			a.Tag, a.Values = tag, append(a.Values, v)
			coll[member] = a
		}
	}
}

// NewGetPrinterAttributes builds a Get-Printer-Attributes request for uri.
func NewGetPrinterAttributes(uri string, requestID uint32, requested []string) *Message {
	ops := []Attribute{
		{Tag: TagCharset, Name: "attributes-charset", Values: []any{"utf-8"}},
		{Tag: TagLanguage, Name: "attributes-natural-language", Values: []any{"en"}},
		{Tag: TagURI, Name: "printer-uri", Values: []any{uri}},
	}
	if len(requested) > 0 {
		vals := make([]any, len(requested))
		for i, s := range requested {
			vals[i] = s
		}
		ops = append(ops, Attribute{Tag: TagKeyword, Name: "requested-attributes", Values: vals})
	}
	return &Message{Version: [2]byte{2, 0}, StatusCode: OpGetPrinterAttributes, RequestID: requestID, Groups: []Group{{Tag: TagOperation, Attributes: ops}}}
}
