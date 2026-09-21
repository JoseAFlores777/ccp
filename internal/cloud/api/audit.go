package api

// El registro de auditoría (§10.5). Es lo único que el servidor puede contar
// sobre una configuración que no puede leer: quién hizo qué y cuándo, con
// detalles que son ids y cuentas, nunca contenido.

import "time"

// MaxAuditEntries es el tope de una página de GET /v1/audit. No es una
// paginación —quien mira una auditoría mira lo último—, es un tope de
// seguridad: el registro crece sin fin.
const MaxAuditEntries = 1000

// AuditEntry es una línea del registro.
type AuditEntry struct {
	ID int64     `json:"id"`
	At time.Time `json:"at"`
	// Device es el equipo que hizo la acción. Vacío cuando no había ninguno
	// todavía (dar de alta el primero, crear la bóveda).
	Device string `json:"device"`
	// DeviceName se resuelve al leer y va vacío si el equipo ya no está: lo
	// apuntado es el id, que es lo que no cambia.
	DeviceName string `json:"device_name"`
	Action     string `json:"action"`
	// Detail son ids y contadores. Nunca configuración: el almacén lo poda al
	// escribir (store.SanitizeAuditDetail).
	Detail map[string]any `json:"detail"`
}
