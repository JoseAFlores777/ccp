#!/usr/bin/env python3
"""Imprime la cadena base64 que acepta Dokploy en «Compose → Advanced → Import».

Dokploy decodifica {"compose": <docker-compose.yml>, "config": <template.toml>}, genera los
secretos de [variables] en el servidor y crea un file mount por cada [[config.mounts]]. Los mounts
se añaden aquí desde files/ para que el realm y el script vivan una sola vez, como archivos reales.
"""
import base64
import json
import pathlib
import sys

here = pathlib.Path(__file__).resolve().parent
compose = (here / "docker-compose.yml").read_text()
config = (here / "template.toml").read_text().rstrip() + "\n"

for f in sorted((here / "files").iterdir()):
    content = f.read_text()
    # Dokploy interpreta el patrón dólar-llave como variable de plantilla, y ''' cerraría el literal TOML.
    for bad in ("${", "'''"):
        if bad in content:
            sys.exit(f"{f.name}: contiene {bad!r}; no puede ir como file mount de plantilla")
    config += f"\n[[config.mounts]]\nfilePath = \"{f.name}\"\ncontent = '''\n{content}'''\n"

print(base64.b64encode(json.dumps({"compose": compose, "config": config}).encode()).decode())
