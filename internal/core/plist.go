package core

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// plist.go — lector/escritor mínimo de property lists XML.
//
// Existe por desktop_app.go: el Info.plist del lanzador se deriva del de
// Claude.app (mismos valores, unos pocos parcheados) y hay que poder hacerlo
// sin `plutil` para que la lógica sea testable en el CI de Linux. Cubre lo que
// un Info.plist trae —dict, array, string, integer, real, true/false, data,
// date— y preserva el ORDEN de las claves, que es lo que hace que un diff del
// plist generado contra el original sea legible.
//
// Un plist binario (`bplist00`) se convierte con `plutil -convert xml1` en
// macOS, que es el único sitio donde puede aparecer uno.

// plistDict es un diccionario con orden de inserción estable.
type plistDict struct {
	keys []string
	vals map[string]any
}

// plistData es el contenido decodificado de un <data>.
type plistData []byte

// plistDate es un <date> tal cual (ISO 8601); no se interpreta.
type plistDate string

func newPlistDict() *plistDict {
	return &plistDict{vals: map[string]any{}}
}

// Get devuelve el valor de k y si existía.
func (d *plistDict) Get(k string) (any, bool) {
	v, ok := d.vals[k]
	return v, ok
}

// String devuelve el valor de k si es un string; "" en cualquier otro caso.
func (d *plistDict) String(k string) string {
	s, _ := d.vals[k].(string)
	return s
}

// Dict devuelve el valor de k si es un diccionario; nil si no.
func (d *plistDict) Dict(k string) *plistDict {
	sub, _ := d.vals[k].(*plistDict)
	return sub
}

// Set fija k; una clave nueva se añade al final para no reordenar el original.
func (d *plistDict) Set(k string, v any) {
	if _, ok := d.vals[k]; !ok {
		d.keys = append(d.keys, k)
	}
	d.vals[k] = v
}

// Delete quita k (no-op si no existe).
func (d *plistDict) Delete(k string) {
	if _, ok := d.vals[k]; !ok {
		return
	}
	delete(d.vals, k)
	for i, kk := range d.keys {
		if kk == k {
			d.keys = append(d.keys[:i], d.keys[i+1:]...)
			break
		}
	}
}

// Keys devuelve las claves en orden.
func (d *plistDict) Keys() []string {
	return append([]string(nil), d.keys...)
}

// Clone copia el diccionario en profundidad.
func (d *plistDict) Clone() *plistDict {
	out := newPlistDict()
	for _, k := range d.keys {
		out.Set(k, clonePlistValue(d.vals[k]))
	}
	return out
}

func clonePlistValue(v any) any {
	switch t := v.(type) {
	case *plistDict:
		return t.Clone()
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = clonePlistValue(e)
		}
		return out
	case plistData:
		return plistData(append([]byte(nil), t...))
	default:
		return v
	}
}

// readPlist lee un Info.plist del disco. Los binarios se convierten con plutil
// (solo macOS); fuera de ahí un bplist es un error explícito, no un parseo
// silenciosamente vacío.
func readPlist(path string) (*plistDict, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(b, []byte("bplist")) {
		if runtime.GOOS != "darwin" {
			return nil, fmt.Errorf("%s es un plist binario y no hay plutil para convertirlo", path)
		}
		out, cerr := exec.Command("plutil", "-convert", "xml1", "-o", "-", path).Output()
		if cerr != nil {
			return nil, fmt.Errorf("plutil no pudo convertir %s: %w", path, cerr)
		}
		b = out
	}
	d, err := parsePlist(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

// parsePlist parsea un plist XML cuyo nodo raíz es un dict.
func parsePlist(b []byte) (*plistDict, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	// El DOCTYPE apunta a apple.com: no se resuelve nada externo.
	dec.Strict = true
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("plist sin nodo <plist>")
			}
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local != "plist" {
			return nil, fmt.Errorf("nodo raíz inesperado <%s>", se.Name.Local)
		}
		break
	}
	root, err := nextPlistStart(dec)
	if err != nil {
		return nil, err
	}
	v, err := parsePlistValue(dec, root)
	if err != nil {
		return nil, err
	}
	d, ok := v.(*plistDict)
	if !ok {
		return nil, fmt.Errorf("el plist no es un dict en la raíz")
	}
	return d, nil
}

// nextPlistStart avanza hasta el siguiente StartElement, saltando espacios,
// comentarios y demás. EOF o un EndElement son error: el llamador esperaba un
// valor.
func nextPlistStart(dec *xml.Decoder) (xml.StartElement, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return xml.StartElement{}, fmt.Errorf("plist truncado: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			return t, nil
		case xml.EndElement:
			return xml.StartElement{}, fmt.Errorf("se esperaba un valor y llegó </%s>", t.Name.Local)
		}
	}
}

// parsePlistValue parsea el valor abierto por start y consume su cierre.
func parsePlistValue(dec *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		d := newPlistDict()
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("dict truncado: %w", err)
			}
			switch t := tok.(type) {
			case xml.EndElement:
				return d, nil
			case xml.StartElement:
				if t.Name.Local != "key" {
					return nil, fmt.Errorf("en un dict se esperaba <key>, llegó <%s>", t.Name.Local)
				}
				key, err := plistText(dec)
				if err != nil {
					return nil, err
				}
				vs, err := nextPlistStart(dec)
				if err != nil {
					return nil, err
				}
				v, err := parsePlistValue(dec, vs)
				if err != nil {
					return nil, err
				}
				d.Set(key, v)
			}
		}
	case "array":
		arr := []any{}
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("array truncado: %w", err)
			}
			switch t := tok.(type) {
			case xml.EndElement:
				return arr, nil
			case xml.StartElement:
				v, err := parsePlistValue(dec, t)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
		}
	case "string":
		return plistText(dec)
	case "date":
		s, err := plistText(dec)
		return plistDate(s), err
	case "data":
		s, err := plistText(dec)
		if err != nil {
			return nil, err
		}
		raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
		if err != nil {
			return nil, fmt.Errorf("<data> inválido: %w", err)
		}
		return plistData(raw), nil
	case "integer":
		s, err := plistText(dec)
		if err != nil {
			return nil, err
		}
		n, perr := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if perr != nil {
			return nil, fmt.Errorf("<integer> inválido %q", s)
		}
		return n, nil
	case "real":
		s, err := plistText(dec)
		if err != nil {
			return nil, err
		}
		f, perr := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if perr != nil {
			return nil, fmt.Errorf("<real> inválido %q", s)
		}
		return f, nil
	case "true", "false":
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return start.Name.Local == "true", nil
	default:
		return nil, fmt.Errorf("tipo de plist no soportado <%s>", start.Name.Local)
	}
}

// plistText lee el texto de un elemento simple y consume su cierre.
func plistText(dec *xml.Decoder) (string, error) {
	var sb strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("elemento truncado: %w", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			sb.Write(t)
		case xml.EndElement:
			return sb.String(), nil
		case xml.StartElement:
			return "", fmt.Errorf("elemento anidado inesperado <%s>", t.Name.Local)
		}
	}
}

// marshalPlist serializa con el mismo formato que plutil -convert xml1 (tabs,
// DOCTYPE de Apple), para que el resultado sea diff-eable contra el original.
func marshalPlist(d *plistDict) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n")
	writePlistValue(&b, d, 0)
	b.WriteString("</plist>\n")
	return b.Bytes()
}

func writePlistValue(b *bytes.Buffer, v any, depth int) {
	ind := strings.Repeat("\t", depth)
	switch t := v.(type) {
	case *plistDict:
		if len(t.keys) == 0 {
			b.WriteString(ind + "<dict/>\n")
			return
		}
		b.WriteString(ind + "<dict>\n")
		for _, k := range t.keys {
			b.WriteString(ind + "\t<key>")
			_ = xml.EscapeText(b, []byte(k))
			b.WriteString("</key>\n")
			writePlistValue(b, t.vals[k], depth+1)
		}
		b.WriteString(ind + "</dict>\n")
	case []any:
		if len(t) == 0 {
			b.WriteString(ind + "<array/>\n")
			return
		}
		b.WriteString(ind + "<array>\n")
		for _, e := range t {
			writePlistValue(b, e, depth+1)
		}
		b.WriteString(ind + "</array>\n")
	case string:
		b.WriteString(ind + "<string>")
		_ = xml.EscapeText(b, []byte(t))
		b.WriteString("</string>\n")
	case plistDate:
		b.WriteString(ind + "<date>" + string(t) + "</date>\n")
	case plistData:
		b.WriteString(ind + "<data>\n" + ind + base64.StdEncoding.EncodeToString(t) + "\n" + ind + "</data>\n")
	case int64:
		b.WriteString(ind + "<integer>" + strconv.FormatInt(t, 10) + "</integer>\n")
	case int:
		b.WriteString(ind + "<integer>" + strconv.Itoa(t) + "</integer>\n")
	case float64:
		b.WriteString(ind + "<real>" + strconv.FormatFloat(t, 'f', -1, 64) + "</real>\n")
	case bool:
		if t {
			b.WriteString(ind + "<true/>\n")
		} else {
			b.WriteString(ind + "<false/>\n")
		}
	default:
		// Un tipo que no sabemos escribir se serializa como string: mejor un
		// valor legible que un plist inválido que LaunchServices rechace.
		b.WriteString(ind + "<string>")
		_ = xml.EscapeText(b, []byte(fmt.Sprint(t)))
		b.WriteString("</string>\n")
	}
}
