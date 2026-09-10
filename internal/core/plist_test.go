package core

import (
	"strings"
	"testing"
)

const testPlistXML = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDisplayName</key>
	<string>Claude</string>
	<key>CFBundleDocumentTypes</key>
	<array>
		<dict>
			<key>CFBundleTypeExtensions</key>
			<array>
				<string>dxt</string>
			</array>
			<key>CFBundleTypeRole</key>
			<string>Viewer</string>
		</dict>
	</array>
	<key>CFBundleExecutable</key>
	<string>Claude</string>
	<key>CFBundleIdentifier</key>
	<string>com.anthropic.claudefordesktop</string>
	<key>ElectronAsarIntegrity</key>
	<dict>
		<key>Resources/app.asar</key>
		<dict>
			<key>algorithm</key>
			<string>SHA256</string>
			<key>hash</key>
			<string>abc123</string>
		</dict>
	</dict>
	<key>LSEnvironment</key>
	<dict>
		<key>MallocNanoZone</key>
		<string>0</string>
	</dict>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSQuitAlwaysKeepsWindows</key>
	<false/>
	<key>SomeInteger</key>
	<integer>42</integer>
	<key>SomeReal</key>
	<real>1.5</real>
	<key>SomeData</key>
	<data>
	aGVsbG8=
	</data>
	<key>SomeDate</key>
	<date>2026-01-02T03:04:05Z</date>
	<key>Escaped &amp; Ampersand</key>
	<string>a &lt; b</string>
</dict>
</plist>
`

// TestPlistRoundTrip: lo que se parsea se vuelve a escribir igual, clave a
// clave y en el mismo orden. Es lo que hace que el Info.plist del lanzador sea
// un diff limpio contra el de Claude.app.
func TestPlistRoundTrip(t *testing.T) {
	d, err := parsePlist([]byte(testPlistXML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := d.String("CFBundleIdentifier"); got != "com.anthropic.claudefordesktop" {
		t.Errorf("CFBundleIdentifier = %q", got)
	}
	if v, _ := d.Get("NSHighResolutionCapable"); v != true {
		t.Errorf("NSHighResolutionCapable = %v", v)
	}
	if v, _ := d.Get("NSQuitAlwaysKeepsWindows"); v != false {
		t.Errorf("NSQuitAlwaysKeepsWindows = %v", v)
	}
	if v, _ := d.Get("SomeInteger"); v != int64(42) {
		t.Errorf("SomeInteger = %v (%T)", v, v)
	}
	if v, _ := d.Get("SomeReal"); v != 1.5 {
		t.Errorf("SomeReal = %v", v)
	}
	if v, _ := d.Get("SomeData"); string(v.(plistData)) != "hello" {
		t.Errorf("SomeData = %q", v)
	}
	if v, _ := d.Get("SomeDate"); v != plistDate("2026-01-02T03:04:05Z") {
		t.Errorf("SomeDate = %v", v)
	}
	if got := d.String("Escaped & Ampersand"); got != "a < b" {
		t.Errorf("escapes: %q", got)
	}
	integ := d.Dict("ElectronAsarIntegrity")
	if integ == nil || integ.Dict("Resources/app.asar") == nil {
		t.Fatal("ElectronAsarIntegrity perdido")
	}
	if got := integ.Dict("Resources/app.asar").String("hash"); got != "abc123" {
		t.Errorf("hash = %q", got)
	}
	docs, _ := d.Get("CFBundleDocumentTypes")
	if arr, ok := docs.([]any); !ok || len(arr) != 1 {
		t.Errorf("CFBundleDocumentTypes = %#v", docs)
	}

	out := string(marshalPlist(d))
	if out != testPlistXML {
		t.Errorf("round-trip difiere:\n--- quería ---\n%s\n--- obtuve ---\n%s", testPlistXML, out)
	}
}

// TestPlistSetDeleteConservaOrden: parchear claves existentes no las mueve, y
// las nuevas van al final.
func TestPlistSetDeleteConservaOrden(t *testing.T) {
	d, err := parsePlist([]byte(testPlistXML))
	if err != nil {
		t.Fatal(err)
	}
	d.Set("CFBundleIdentifier", "com.example.x")
	d.Delete("CFBundleDocumentTypes")
	d.Set("CCPNueva", "v")

	keys := d.Keys()
	if keys[0] != "CFBundleDisplayName" || keys[1] != "CFBundleExecutable" {
		t.Errorf("orden tras Delete: %v", keys[:3])
	}
	if keys[len(keys)-1] != "CCPNueva" {
		t.Errorf("la clave nueva debería ir al final: %v", keys)
	}
	if _, ok := d.Get("CFBundleDocumentTypes"); ok {
		t.Error("Delete no quitó la clave")
	}
	out := string(marshalPlist(d))
	if !strings.Contains(out, "<string>com.example.x</string>") || strings.Contains(out, "CFBundleDocumentTypes") {
		t.Errorf("serialización tras parche:\n%s", out)
	}
}

// TestPlistCloneEsProfundo: el Clone es lo que separa el plist del lanzador
// del original leído; mutar uno no puede tocar el otro.
func TestPlistCloneEsProfundo(t *testing.T) {
	d, _ := parsePlist([]byte(testPlistXML))
	c := d.Clone()
	c.Dict("ElectronAsarIntegrity").Set("otro", "x")
	if _, ok := d.Dict("ElectronAsarIntegrity").Get("otro"); ok {
		t.Error("Clone compartió el sub-dict")
	}
}

// TestPlistRechazaLoQueNoEsDict: un plist cuya raíz no es dict no es un
// Info.plist, y un XML cualquiera tampoco.
func TestPlistRechazaLoQueNoEsDict(t *testing.T) {
	if _, err := parsePlist([]byte(`<?xml version="1.0"?><plist version="1.0"><array/></plist>`)); err == nil {
		t.Error("raíz array debería fallar")
	}
	if _, err := parsePlist([]byte(`<html></html>`)); err == nil {
		t.Error("XML ajeno debería fallar")
	}
	if _, err := parsePlist([]byte(`<plist version="1.0"><dict><key>a</key>`)); err == nil {
		t.Error("truncado debería fallar")
	}
}
