package store

import (
	"context"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// El Store de producción tiene que cumplir la misma interfaz que el de memoria:
// si no, el contrato compartido no dice nada sobre él.
var _ Store = (*PG)(nil)

// Migrate parsea la versión del nombre del archivo con Atoi y aplica en orden
// alfabético. Un nombre sin número por delante rompe el arranque del servidor,
// no un test de Postgres: por eso se comprueba aquí, sin base de datos.
func TestMigracionesEmbebidasTienenVersionCreciente(t *testing.T) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no hay migraciones embebidas")
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	prev := 0
	for _, n := range names {
		if !strings.HasSuffix(n, ".sql") {
			t.Fatalf("migración que no es .sql: %s", n)
		}
		v, err := strconv.Atoi(strings.SplitN(n, "_", 2)[0])
		if err != nil {
			t.Fatalf("migración con nombre inválido: %s", n)
		}
		if v <= prev {
			t.Fatalf("la versión de %s no es mayor que la anterior (%d)", n, prev)
		}
		prev = v
		sql, err := migrationFS.ReadFile("migrations/" + n)
		if err != nil || len(sql) == 0 {
			t.Fatalf("migración vacía o ilegible: %s (%v)", n, err)
		}
	}
}

// La garantía que el esquema aporta y el código no puede: una referencia a un
// blob sin registrar la rechaza la clave foránea, dentro de la transacción.
func TestEsquemaInicialAtaLasReferenciasAUnBlobRegistrado(t *testing.T) {
	sql, err := migrationFS.ReadFile("migrations/0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"FOREIGN KEY (user_id, blob_id) REFERENCES blobs (user_id, id)",
		"FOREIGN KEY (user_id, snapshot_id) REFERENCES snapshots (user_id, id)",
	} {
		if !strings.Contains(string(sql), want) {
			t.Fatalf("0001_init.sql no ata %q", want)
		}
	}
}

// La DSN es de palabras clave y la contraseña va aparte; una DSN rota tiene que
// fallar al abrir, no al primer query.
func TestOpenPGRechazaDSNInvalida(t *testing.T) {
	if _, err := OpenPG(context.Background(), "esto no es una dsn", ""); err == nil {
		t.Fatal("OpenPG aceptó una DSN inválida")
	}
}
