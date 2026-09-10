package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"strings"
)

// icns.go — el icono del lanzador: el de Claude.app con otro color.
//
// Un .icns moderno es un contenedor de trozos y los tamaños grandes (ic07…ic14)
// son PNG tal cual. Se decodifica cada PNG, se rota el matiz de los píxeles
// saturados (el naranja del fondo) dejando intactos los grises y el alfa (el
// asterisco crema, los bordes), y se vuelve a empaquetar. Los trozos que no son
// PNG (ic04/ic05 ARGB, TOC, info) se descartan: macOS escala desde los que
// quedan y así no hay que implementar el RLE de Apple.

// DesktopColor es un tinte aplicable al icono.
type DesktopColor struct {
	Name     string  // nombre de la paleta o "#rrggbb"
	Hue      float64 // matiz destino, grados [0,360)
	Sat      float64 // -1: conservar la saturación; 0: desaturar; >0: saturación objetivo (hex)
	Identity bool    // true = dejar el icono como está ("orange")
}

// DesktopPalette son los colores con nombre, en el orden en que se asignan
// automáticamente. El naranja original va último: es el del perfil default y
// elegirlo para un lanzador lo haría indistinguible de la app normal.
var DesktopPalette = []DesktopColor{
	{Name: "blue", Hue: 215, Sat: -1},
	{Name: "green", Hue: 140, Sat: -1},
	{Name: "purple", Hue: 272, Sat: -1},
	{Name: "pink", Hue: 330, Sat: -1},
	{Name: "teal", Hue: 182, Sat: -1},
	{Name: "yellow", Hue: 46, Sat: -1},
	{Name: "red", Hue: 352, Sat: -1},
	{Name: "gray", Hue: 0, Sat: 0},
	{Name: "orange", Identity: true},
}

// ParseDesktopColor acepta un nombre de la paleta (sin distinguir mayúsculas)
// o un hex CSS (#rgb / #rrggbb), del que toma matiz y saturación.
func ParseDesktopColor(s string) (DesktopColor, error) {
	name := strings.ToLower(strings.TrimSpace(s))
	if name == "" {
		return DesktopColor{}, fmt.Errorf("color vacío")
	}
	for _, c := range DesktopPalette {
		if c.Name == name {
			return c, nil
		}
	}
	if strings.HasPrefix(name, "#") {
		hex := name[1:]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) != 6 {
			return DesktopColor{}, fmt.Errorf("hex inválido %q (usa #rrggbb)", s)
		}
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return DesktopColor{}, fmt.Errorf("hex inválido %q: %w", s, err)
		}
		r := float64(v>>16&0xff) / 255
		g := float64(v>>8&0xff) / 255
		b := float64(v&0xff) / 255
		h, sat, _ := rgbToHSL(r, g, b)
		return DesktopColor{Name: "#" + hex, Hue: h, Sat: sat}, nil
	}
	names := make([]string, 0, len(DesktopPalette))
	for _, c := range DesktopPalette {
		names = append(names, c.Name)
	}
	return DesktopColor{}, fmt.Errorf("color desconocido %q (paleta: %s, o #rrggbb)", s, strings.Join(names, ", "))
}

// DesktopPaletteNames devuelve los nombres de la paleta en orden.
func DesktopPaletteNames() []string {
	out := make([]string, 0, len(DesktopPalette))
	for _, c := range DesktopPalette {
		out = append(out, c.Name)
	}
	return out
}

// icnsChunk es un trozo del contenedor sin su cabecera de 8 bytes.
type icnsChunk struct {
	typ  string
	data []byte
}

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// icnsSplit descompone un .icns en trozos. Es estricto con la estructura: un
// contenedor corrupto tiene que fallar aquí, no producir un icono a medias que
// LaunchServices ignore en silencio.
func icnsSplit(b []byte) ([]icnsChunk, error) {
	if len(b) < 8 || string(b[:4]) != "icns" {
		return nil, fmt.Errorf("no es un .icns")
	}
	end := int(binary.BigEndian.Uint32(b[4:8]))
	if end > len(b) || end < 8 {
		return nil, fmt.Errorf(".icns truncado (declara %d bytes, hay %d)", end, len(b))
	}
	var chunks []icnsChunk
	off := 8
	for off+8 <= end {
		l := int(binary.BigEndian.Uint32(b[off+4 : off+8]))
		if l < 8 || off+l > end {
			return nil, fmt.Errorf(".icns corrupto en el trozo %q", b[off:off+4])
		}
		chunks = append(chunks, icnsChunk{typ: string(b[off : off+4]), data: b[off+8 : off+l]})
		off += l
	}
	return chunks, nil
}

// icnsJoin es el inverso de icnsSplit.
func icnsJoin(chunks []icnsChunk) []byte {
	total := 8
	for _, c := range chunks {
		total += 8 + len(c.data)
	}
	out := make([]byte, 0, total)
	out = append(out, "icns"...)
	out = binary.BigEndian.AppendUint32(out, uint32(total))
	for _, c := range chunks {
		out = append(out, c.typ...)
		out = binary.BigEndian.AppendUint32(out, uint32(8+len(c.data)))
		out = append(out, c.data...)
	}
	return out
}

// TintICNS devuelve una copia de src con el color aplicado. Con un color
// Identity devuelve src tal cual (mismos bytes: el icono original).
func TintICNS(src []byte, c DesktopColor) ([]byte, error) {
	if c.Identity {
		return append([]byte(nil), src...), nil
	}
	chunks, err := icnsSplit(src)
	if err != nil {
		return nil, err
	}

	var out []icnsChunk
	var shift, satScale float64
	var calibrated bool
	for _, ch := range chunks {
		if !bytes.HasPrefix(ch.data, pngMagic) {
			continue
		}
		img, derr := png.Decode(bytes.NewReader(ch.data))
		if derr != nil {
			return nil, fmt.Errorf("trozo %q: PNG inválido: %w", ch.typ, derr)
		}
		nrgba := toNRGBA(img)
		if !calibrated {
			// La rotación se calibra una sola vez, sobre el primer PNG: todos
			// los tamaños son el mismo dibujo y el matiz dominante es el mismo.
			domHue, domSat, ok := dominantHue(nrgba)
			shift, satScale = 0, 1
			if ok {
				shift = c.Hue - domHue
			}
			switch {
			case c.Sat == 0:
				satScale = 0
			case c.Sat > 0 && ok && domSat > 0.05:
				satScale = math.Min(c.Sat/domSat, 1.5)
			}
			calibrated = true
		}
		tintImage(nrgba, shift, satScale)
		var buf bytes.Buffer
		if err := png.Encode(&buf, nrgba); err != nil {
			return nil, err
		}
		out = append(out, icnsChunk{typ: ch.typ, data: buf.Bytes()})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("el .icns no tiene trozos PNG que tintar")
	}
	return icnsJoin(out), nil
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	n := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(n, n.Bounds(), img, b.Min, draw.Src)
	return n
}

// dominantHue estima el matiz del área saturada del icono (el fondo) con una
// media circular ponderada por saturación, y devuelve también su saturación
// media. ok=false si no hay píxeles saturados (icono monocromo).
func dominantHue(n *image.NRGBA) (hue, sat float64, ok bool) {
	var sx, sy, sumSat, count float64
	for i := 0; i+3 < len(n.Pix); i += 4 {
		a := n.Pix[i+3]
		if a < 128 {
			continue
		}
		h, s, l := rgbToHSL(float64(n.Pix[i])/255, float64(n.Pix[i+1])/255, float64(n.Pix[i+2])/255)
		if s < 0.3 || l < 0.12 || l > 0.92 {
			continue
		}
		rad := h * math.Pi / 180
		sx += math.Cos(rad) * s
		sy += math.Sin(rad) * s
		sumSat += s
		count++
	}
	if count == 0 {
		return 0, 0, false
	}
	hue = math.Atan2(sy, sx) * 180 / math.Pi
	if hue < 0 {
		hue += 360
	}
	return hue, sumSat / count, true
}

// tintImage rota el matiz de los píxeles saturados y escala su saturación, in
// place. Alfa y luminosidad no se tocan: el dibujo es el mismo, cambia el color.
func tintImage(n *image.NRGBA, shift, satScale float64) {
	for i := 0; i+3 < len(n.Pix); i += 4 {
		if n.Pix[i+3] == 0 {
			continue
		}
		h, s, l := rgbToHSL(float64(n.Pix[i])/255, float64(n.Pix[i+1])/255, float64(n.Pix[i+2])/255)
		if s < 0.05 {
			continue // gris: no tiene matiz que rotar
		}
		h = math.Mod(h+shift+360, 360)
		s = math.Max(0, math.Min(1, s*satScale))
		r, g, b := hslToRGB(h, s, l)
		n.Pix[i] = uint8(math.Round(r * 255))
		n.Pix[i+1] = uint8(math.Round(g * 255))
		n.Pix[i+2] = uint8(math.Round(b * 255))
	}
}

func rgbToHSL(r, g, b float64) (h, s, l float64) {
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h * 60, s, l
}

func hslToRGB(h, s, l float64) (r, g, b float64) {
	if s == 0 {
		return l, l, l
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	hk := h / 360
	return hueToRGB(p, q, hk+1.0/3), hueToRGB(p, q, hk), hueToRGB(p, q, hk-1.0/3)
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 0.5:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}
