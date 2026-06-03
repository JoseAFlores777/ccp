# Security Policy

## Supported versions

| Version | Supported |
| ------- | --------- |
| 2.6.x   | ✅        |
| < 2.6   | ❌        |

Only the latest minor release receives security fixes. Upgrade with `ccp upgrade`.

## Reporting a vulnerability

**Do not open a public issue for security problems.**

Report privately to **joseadolfoizaguirreflores@gmail.com** with:

- a description of the issue and its impact,
- steps to reproduce (or a proof of concept),
- affected version (`ccp version`) and platform.

You can expect an acknowledgement within a few days. Once a fix is released,
the report may be disclosed publicly with credit to the reporter (unless you
prefer to remain anonymous).

## Scope notes

`ccp` stores provider API keys under `~/.config/ccp/profiles/<name>/api_key`
(`chmod 600`) and never writes them to `ccp.yaml`, the shell rc, or git.
Reports about key handling, the shell-init eval surface (`ccp _env` / `ccp _hook`),
or path-resolution leaks are especially welcome.
