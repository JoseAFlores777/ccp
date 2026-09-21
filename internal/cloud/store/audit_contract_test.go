package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// runAuditContract cubre el registro de auditoría de §10.5: solo inserta, se
// lee del más nuevo al más viejo y NUNCA guarda configuración. Va aparte con
// su propio usuario porque el registro es una historia entera.
func runAuditContract(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	u, _ := s.UpsertUser(ctx, "sub-audit", "audit@x")
	other, _ := s.UpsertUser(ctx, "sub-audit-otro", "otro@x")
	d, _ := s.CreateDevice(ctx, u.ID, Device{Name: "mac"})
	d2, _ := s.CreateDevice(ctx, u.ID, Device{Name: "linux"})

	for _, e := range []struct {
		dev    string
		action string
		detail map[string]any
	}{
		{d.ID, "device.create", map[string]any{"name": "mac"}},
		{d.ID, "snapshot.commit", map[string]any{"id": hexID('a'), "blobs": 3}},
		{d2.ID, "revision.publish", map[string]any{"device": d.ID}},
		{"", "vault.create", nil},
	} {
		if err := s.Audit(ctx, u.ID, e.dev, e.action, e.detail); err != nil {
			t.Fatalf("Audit(%s): %v", e.action, err)
		}
	}
	if err := s.Audit(ctx, other.ID, "", "vault.create", nil); err != nil {
		t.Fatal(err)
	}

	all, err := s.AuditLog(ctx, u.ID, AuditFilter{})
	if err != nil || len(all) != 4 {
		t.Fatalf("AuditLog = %d entradas, %v", len(all), err)
	}
	// Del más nuevo al más viejo: quien mira una auditoría mira lo último.
	if all[0].Action != "vault.create" || all[3].Action != "device.create" {
		t.Fatalf("orden = %s … %s", all[0].Action, all[3].Action)
	}
	if all[0].At.IsZero() || all[0].ID == 0 {
		t.Fatalf("una entrada sin id ni fecha no es un registro: %+v", all[0])
	}
	// El nombre del equipo se resuelve al leer: el registro guarda su id.
	if all[3].DeviceID != d.ID || all[3].DeviceName != "mac" {
		t.Fatalf("equipo = %+v", all[3])
	}
	if all[0].DeviceID != "" || all[0].DeviceName != "" {
		t.Fatalf("hay cosas que apuntar sin equipo: %+v", all[0])
	}
	if all[2].Detail["blobs"] == nil || all[2].Detail["id"] != hexID('a') {
		t.Fatalf("detalle = %+v", all[2].Detail)
	}

	// Acotado al usuario: la auditoría de una cuenta no se ve desde otra.
	ajena, err := s.AuditLog(ctx, other.ID, AuditFilter{})
	if err != nil || len(ajena) != 1 {
		t.Fatalf("AuditLog ajena = %d, %v", len(ajena), err)
	}

	porEquipo, err := s.AuditLog(ctx, u.ID, AuditFilter{DeviceID: d.ID})
	if err != nil || len(porEquipo) != 2 {
		t.Fatalf("filtro por equipo = %d, %v", len(porEquipo), err)
	}
	porAccion, err := s.AuditLog(ctx, u.ID, AuditFilter{Action: "snapshot.commit"})
	if err != nil || len(porAccion) != 1 || porAccion[0].Action != "snapshot.commit" {
		t.Fatalf("filtro por acción = %+v, %v", porAccion, err)
	}
	if n, _ := s.AuditLog(ctx, u.ID, AuditFilter{Limit: 2}); len(n) != 2 {
		t.Fatalf("límite = %d", len(n))
	}
	// Since es un corte por fecha: en el futuro no hay nada apuntado.
	futuro, err := s.AuditLog(ctx, u.ID, AuditFilter{Since: time.Now().UTC().Add(time.Hour)})
	if err != nil || len(futuro) != 0 {
		t.Fatalf("desde el futuro = %d, %v", len(futuro), err)
	}
	if _, err := s.AuditLog(ctx, "no-es-un-uuid", AuditFilter{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("usuario inexistente: %v", err)
	}

	// Lo que no cabe en un id no es un detalle, es una fuga: el registro dice
	// QUÉ pasó, nunca qué configuración había dentro.
	if err := s.Audit(ctx, u.ID, d.ID, "revision.publish", map[string]any{
		"id":       hexID('b'),
		"manifest": strings.Repeat("x", 4096),
		"body":     map[string]any{"env": map[string]any{"TOKEN": "sk-secreto"}},
	}); err != nil {
		t.Fatal(err)
	}
	last, _ := s.AuditLog(ctx, u.ID, AuditFilter{Limit: 1})
	if len(last) != 1 {
		t.Fatalf("AuditLog = %d", len(last))
	}
	if s, _ := last[0].Detail["manifest"].(string); len(s) > MaxAuditDetailLen {
		t.Fatalf("un detalle largo se recorta: %d bytes", len(s))
	}
	if _, anidado := last[0].Detail["body"]; anidado {
		t.Fatalf("el detalle anidado no se guarda: %+v", last[0].Detail)
	}
	if last[0].Detail["id"] != hexID('b') {
		t.Fatalf("recortar no puede llevarse los ids: %+v", last[0].Detail)
	}
}
