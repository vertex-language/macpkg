package plist

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Header is the standard Apple XML Plist doctype header.
const Header = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
`

const Footer = `</plist>
`

// MarshalXML formats v as an Apple XML Property List.
func MarshalXML(v any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(Header)
	if err := encodeValue(&buf, v, 0); err != nil {
		return nil, err
	}
	buf.WriteString(Footer)
	return buf.Bytes(), nil
}

func indent(w io.Writer, depth int) {
	for i := 0; i < depth; i++ {
		w.Write([]byte("\t"))
	}
}

func encodeValue(w *bytes.Buffer, v any, depth int) error {
	indent(w, depth)
	switch val := v.(type) {
	case string:
		var esc bytes.Buffer
		xml.EscapeText(&esc, []byte(val))
		fmt.Fprintf(w, "<string>%s</string>\n", esc.String())
	case bool:
		if val {
			w.WriteString("<true/>\n")
		} else {
			w.WriteString("<false/>\n")
		}
	case int:
		fmt.Fprintf(w, "<integer>%d</integer>\n", val)
	case int64:
		fmt.Fprintf(w, "<integer>%d</integer>\n", val)
	case uint:
		fmt.Fprintf(w, "<integer>%d</integer>\n", val)
	case uint64:
		fmt.Fprintf(w, "<integer>%d</integer>\n", val)
	case float64:
		fmt.Fprintf(w, "<real>%g</real>\n", val)
	case []byte:
		b64 := base64.StdEncoding.EncodeToString(val)
		fmt.Fprintf(w, "<data>%s</data>\n", b64)
	case time.Time:
		fmt.Fprintf(w, "<date>%s</date>\n", val.UTC().Format(time.RFC3339))
	case []string:
		w.WriteString("<array>\n")
		for _, item := range val {
			if err := encodeValue(w, item, depth+1); err != nil {
				return err
			}
		}
		indent(w, depth)
		w.WriteString("</array>\n")
	case []any:
		w.WriteString("<array>\n")
		for _, item := range val {
			if err := encodeValue(w, item, depth+1); err != nil {
				return err
			}
		}
		indent(w, depth)
		w.WriteString("</array>\n")
	case map[string]string:
		w.WriteString("<dict>\n")
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			indent(w, depth+1)
			var escKey bytes.Buffer
			xml.EscapeText(&escKey, []byte(k))
			fmt.Fprintf(w, "<key>%s</key>\n", escKey.String())
			if err := encodeValue(w, val[k], depth+1); err != nil {
				return err
			}
		}
		indent(w, depth)
		w.WriteString("</dict>\n")
	case map[string]any:
		w.WriteString("<dict>\n")
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			indent(w, depth+1)
			var escKey bytes.Buffer
			xml.EscapeText(&escKey, []byte(k))
			fmt.Fprintf(w, "<key>%s</key>\n", escKey.String())
			if err := encodeValue(w, val[k], depth+1); err != nil {
				return err
			}
		}
		indent(w, depth)
		w.WriteString("</dict>\n")
	default:
		return fmt.Errorf("unsupported plist value type: %T", v)
	}
	return nil
}

// UnmarshalXML parses Apple XML Property List into map[string]any or a target.
func UnmarshalXML(data []byte) (map[string]any, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "plist" {
				// Find root value
				val, err := parseElement(decoder)
				if err != nil {
					return nil, err
				}
				if m, ok := val.(map[string]any); ok {
					return m, nil
				}
				return nil, fmt.Errorf("plist root is not a dict, got %T", val)
			}
		}
	}
}

func parseElement(d *xml.Decoder) (any, error) {
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch elem := tok.(type) {
		case xml.StartElement:
			switch elem.Name.Local {
			case "string":
				var s string
				if err := d.DecodeElement(&s, &elem); err != nil {
					return nil, err
				}
				return s, nil
			case "integer":
				var s string
				if err := d.DecodeElement(&s, &elem); err != nil {
					return nil, err
				}
				n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
				if err != nil {
					return nil, err
				}
				return n, nil
			case "real":
				var s string
				if err := d.DecodeElement(&s, &elem); err != nil {
					return nil, err
				}
				f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
				if err != nil {
					return nil, err
				}
				return f, nil
			case "true":
				_ = d.Skip()
				return true, nil
			case "false":
				_ = d.Skip()
				return false, nil
			case "date":
				var s string
				if err := d.DecodeElement(&s, &elem); err != nil {
					return nil, err
				}
				t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
				if err != nil {
					return s, nil
				}
				return t, nil
			case "data":
				var s string
				if err := d.DecodeElement(&s, &elem); err != nil {
					return nil, err
				}
				cleaned := strings.Join(strings.Fields(s), "")
				return base64.StdEncoding.DecodeString(cleaned)
			case "array":
				var arr []any
				for {
					tok2, err := d.Token()
					if err != nil {
						return nil, err
					}
					if end, ok := tok2.(xml.EndElement); ok && end.Name.Local == "array" {
						break
					}
					if se, ok := tok2.(xml.StartElement); ok {
						val, err := parseStart(d, se)
						if err != nil {
							return nil, err
						}
						arr = append(arr, val)
					}
				}
				return arr, nil
			case "dict":
				dict := make(map[string]any)
				var curKey string
				for {
					tok2, err := d.Token()
					if err != nil {
						return nil, err
					}
					if end, ok := tok2.(xml.EndElement); ok && end.Name.Local == "dict" {
						break
					}
					if se, ok := tok2.(xml.StartElement); ok {
						if se.Name.Local == "key" {
							var k string
							if err := d.DecodeElement(&k, &se); err != nil {
								return nil, err
							}
							curKey = k
						} else {
							val, err := parseStart(d, se)
							if err != nil {
								return nil, err
							}
							if curKey != "" {
								dict[curKey] = val
								curKey = ""
							}
						}
					}
				}
				return dict, nil
			}
		case xml.EndElement:
			return nil, nil
		}
	}
}

func parseStart(d *xml.Decoder, se xml.StartElement) (any, error) {
	switch se.Name.Local {
	case "string":
		var s string
		if err := d.DecodeElement(&s, &se); err != nil {
			return nil, err
		}
		return s, nil
	case "integer":
		var s string
		if err := d.DecodeElement(&s, &se); err != nil {
			return nil, err
		}
		return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	case "real":
		var s string
		if err := d.DecodeElement(&s, &se); err != nil {
			return nil, err
		}
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	case "true":
		_ = d.Skip()
		return true, nil
	case "false":
		_ = d.Skip()
		return false, nil
	case "data":
		var s string
		if err := d.DecodeElement(&s, &se); err != nil {
			return nil, err
		}
		return base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
	case "date":
		var s string
		if err := d.DecodeElement(&s, &se); err != nil {
			return nil, err
		}
		return time.Parse(time.RFC3339, strings.TrimSpace(s))
	case "array":
		var arr []any
		for {
			t, err := d.Token()
			if err != nil {
				return nil, err
			}
			if end, ok := t.(xml.EndElement); ok && end.Name.Local == "array" {
				break
			}
			if start, ok := t.(xml.StartElement); ok {
				v, err := parseStart(d, start)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
		}
		return arr, nil
	case "dict":
		dict := make(map[string]any)
		var curKey string
		for {
			t, err := d.Token()
			if err != nil {
				return nil, err
			}
			if end, ok := t.(xml.EndElement); ok && end.Name.Local == "dict" {
				break
			}
			if start, ok := t.(xml.StartElement); ok {
				if start.Name.Local == "key" {
					var k string
					if err := d.DecodeElement(&k, &start); err != nil {
						return nil, err
					}
					curKey = k
				} else {
					v, err := parseStart(d, start)
					if err != nil {
						return nil, err
					}
					if curKey != "" {
						dict[curKey] = v
						curKey = ""
					}
				}
			}
		}
		return dict, nil
	default:
		_ = d.Skip()
		return nil, nil
	}
}

// Lookup decodes the XML plist and returns the value for the given key in the root dict.
func Lookup(data []byte, key string) (any, bool) {
	m, err := UnmarshalXML(data)
	if err != nil {
		return nil, false
	}
	v, ok := m[key]
	return v, ok
}

