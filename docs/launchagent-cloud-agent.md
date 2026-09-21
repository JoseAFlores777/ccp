# Running `ccp cloud agent` in the background (optional)

`ccp cloud agent` applies the revisions your portal publishes for this machine. You can run it by hand
whenever you like; if you want it always on, macOS launches it for you with a **LaunchAgent**.

**ccp does not install this for you, on purpose.** A job that wakes up on its own and rewrites your
configuration is a decision, not a detail an installer should take for you — and one you should be able to
see, read and delete in one file you own. So: copy the plist, load it, and it is yours.

## Before anything else

The agent needs the vault unlocked on this machine (`ccp cloud unlock` once) — a revision arrives sealed and
signed, and without the account key there is nothing it can open or verify. It also needs a policy you are
comfortable with:

```bash
ccp cloud policy            # auto by default: only what does not run code applies on its own
ccp cloud policy manual     # or: nothing applies without ccp cloud review
ccp cloud agent --once      # try one pass by hand first
```

Anything that runs code here still waits for `ccp cloud review` in a terminal, LaunchAgent or not. A
background job never confirms a hook for you.

## The plist

Save it as `~/Library/LaunchAgents/com.ccp.cloud-agent.plist`, with **your** absolute paths (`which ccp`, and
your own home in the log paths):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>              <string>com.ccp.cloud-agent</string>
  <key>ProgramArguments</key>
  <array>
    <string>/Users/CAMBIAME/.local/bin/ccp</string>
    <string>cloud</string>
    <string>agent</string>
    <string>--interval</string>
    <string>5m</string>
  </array>
  <key>RunAtLoad</key>          <true/>
  <key>KeepAlive</key>
  <dict>
    <!-- Restart it if it dies, but not if it exited cleanly: `agent` only
         returns 0 when something stopped it on purpose, and relaunching it
         there would be arguing with you. -->
    <key>SuccessfulExit</key>   <false/>
  </dict>
  <key>ProcessType</key>        <string>Background</string>
  <key>StandardOutPath</key>    <string>/Users/CAMBIAME/Library/Logs/ccp-cloud-agent.log</string>
  <key>StandardErrorPath</key>  <string>/Users/CAMBIAME/Library/Logs/ccp-cloud-agent.log</string>
</dict>
</plist>
```

```bash
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.ccp.cloud-agent.plist
launchctl print gui/$(id -u)/com.ccp.cloud-agent | head        # is it alive?
tail -f ~/Library/Logs/ccp-cloud-agent.log                     # what it is doing
```

To stop it, or to get rid of it:

```bash
launchctl bootout gui/$(id -u)/com.ccp.cloud-agent
rm ~/Library/LaunchAgents/com.ccp.cloud-agent.plist
```

## Things worth knowing before you load it

- **The agent polls; it does not hold a connection open.** Every `--interval` it asks once and goes back to
  sleep, so «5m» means you may learn about a revision up to five minutes late. That is the whole cost: nothing
  is lost in the meantime, and no port is ever opened on this machine.
- **`PATH` is not your shell's.** launchd gives a job a minimal environment, which is why the plist names the
  binary by absolute path. If you use a `CCP_HOME` other than the default, add it under `EnvironmentVariables`
  — the agent reads the same configuration a terminal does.
- **Network errors do not kill it.** A pass that fails is written to the log and the next one tries again; a
  background agent that dies on the first Wi-Fi drop would stop applying for ever without telling anyone.
- **The log is yours to rotate.** ccp writes nothing to it but the same lines you would see in a terminal, and
  never a secret — but it grows.
- **If you would rather not have it**, `ccp cloud agent --once` in a terminal, or the app while it is open,
  does exactly the same work.
