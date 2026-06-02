// Package core es el motor de ccp: CRUD de perfiles, reglas de ruta,
// delta de entorno, overlay de config, store YAML, backup y migración.
//
// core no imprime presentación: devuelve datos y strings; los front-ends
// (internal/cli, internal/tui) formatean. Las excepciones son env.go y
// shellinit.go, que producen strings exactos porque SON el contrato shell.
package core

// Version es la versión del binario ccp (rewrite Go v2.0).
//
// Es var, no const, para que el release la inyecte desde el tag con
// -ldflags "-X github.com/JoseAFlores777/ccp/internal/core.Version=2.1.0".
// El default es el de los builds de desarrollo / desde fuente sin tag.
var Version = "2.0.0"
