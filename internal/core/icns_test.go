package core

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
)

// icnsDePrueba construye un .icns con un PNG naranja (fondo), un píxel blanco,
// uno transparente, más un trozo ARGB y un TOC que NO son PNG.
func icnsDePrueba(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	orange := color.NRGBA{R: 0xd9, G: 0x77, B: 0x57, A: 0xff} // matiz ≈ 15°
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetNRGBA(x, y, orange)
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	img.SetNRGBA(1, 0, color.NRGBA{A: 0})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return icnsJoin([]icnsChunk{
		{typ: "TOC ", data: []byte("xxxxxxxx")},
		{typ: "ic07", data: buf.Bytes()},
		{typ: "ic04", data: []byte("ARGB....")},
	})
}

func hueDe(t *testing.T, c color.NRGBA) (h, s float64) {
	t.Helper()
	h, s, _ = rgbToHSL(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
	return h, s
}

func decodeUnicoPNG(t *testing.T, icns []byte) *image.NRGBA {
	t.Helper()
	chunks, err := icnsSplit(icns)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0].typ != "ic07" {
		t.Fatalf("esperaba solo el trozo PNG ic07, hay %d: %+v", len(chunks), chunks)
	}
	img, err := png.Decode(bytes.NewReader(chunks[0].data))
	if err != nil {
		t.Fatal(err)
	}
	return toNRGBA(img)
}

// TestTintICNSRotaElMatizYRespetaGrisesYAlfa es el contrato visual: el fondo
// cambia de color, el dibujo (blanco) y la transparencia no.
func TestTintICNSRotaElMatizYRespetaGrisesYAlfa(t *testing.T) {
	src := icnsDePrueba(t)
	blue, _ := ParseDesktopColor("blue")
	out, err := TintICNS(src, blue)
	if err != nil {
		t.Fatal(err)
	}
	img := decodeUnicoPNG(t, out)

	h, s := hueDe(t, img.NRGBAAt(4, 4))
	if math.Abs(h-215) > 3 {
		t.Errorf("matiz del fondo = %.1f, quería ≈215 (azul)", h)
	}
	if s < 0.3 {
		t.Errorf("la saturación debería conservarse, dio %.2f", s)
	}
	if w := img.NRGBAAt(0, 0); w.R != 0xff || w.G != 0xff || w.B != 0xff {
		t.Errorf("el blanco cambió: %+v", w)
	}
	if a := img.NRGBAAt(1, 0).A; a != 0 {
		t.Errorf("el alfa cambió: %d", a)
	}
}

func TestTintICNSGrisDesatura(t *testing.T) {
	gray, _ := ParseDesktopColor("gray")
	out, err := TintICNS(icnsDePrueba(t), gray)
	if err != nil {
		t.Fatal(err)
	}
	_, s := hueDe(t, decodeUnicoPNG(t, out).NRGBAAt(4, 4))
	if s > 0.02 {
		t.Errorf("gray debería desaturar, dio s=%.2f", s)
	}
}

func TestTintICNSHexUsaSuMatiz(t *testing.T) {
	red, err := ParseDesktopColor("#ff0000")
	if err != nil {
		t.Fatal(err)
	}
	out, err := TintICNS(icnsDePrueba(t), red)
	if err != nil {
		t.Fatal(err)
	}
	h, _ := hueDe(t, decodeUnicoPNG(t, out).NRGBAAt(4, 4))
	if h > 3 && h < 357 {
		t.Errorf("matiz = %.1f, quería ≈0 (rojo)", h)
	}
}

// TestTintICNSIdentityDevuelveElOriginal: "orange" es el icono de siempre,
// byte a byte — no una re-codificación que pese distinto.
func TestTintICNSIdentityDevuelveElOriginal(t *testing.T) {
	src := icnsDePrueba(t)
	orange, _ := ParseDesktopColor("orange")
	out, err := TintICNS(src, orange)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, src) {
		t.Error("identity debería devolver los mismos bytes")
	}
}

func TestParseDesktopColor(t *testing.T) {
	if _, err := ParseDesktopColor("magenta-ish"); err == nil {
		t.Error("nombre desconocido debería fallar")
	}
	if _, err := ParseDesktopColor("#12"); err == nil {
		t.Error("hex corto debería fallar")
	}
	c, err := ParseDesktopColor("#0F0")
	if err != nil || c.Name != "#00ff00" || math.Abs(c.Hue-120) > 0.5 {
		t.Errorf("#0F0 → %+v, %v", c, err)
	}
	if c, _ := ParseDesktopColor(" Blue "); c.Name != "blue" {
		t.Errorf("nombre con espacios/mayúsculas → %+v", c)
	}
}

func TestIcnsSplitRechazaBasura(t *testing.T) {
	if _, err := icnsSplit([]byte("nope")); err == nil {
		t.Error("magic inválido debería fallar")
	}
	bad := icnsJoin([]icnsChunk{{typ: "ic07", data: []byte("x")}})
	bad[12] = 0xff // longitud del trozo desbordada
	if _, err := icnsSplit(bad); err == nil {
		t.Error("trozo desbordado debería fallar")
	}
	if _, err := TintICNS(icnsJoin([]icnsChunk{{typ: "ic04", data: []byte("ARGB")}}), DesktopPalette[0]); err == nil {
		t.Error("sin trozos PNG debería fallar")
	}
}
