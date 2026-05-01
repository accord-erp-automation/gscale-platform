# bridge

`bridge` stores the shared runtime snapshot that ties scale, printer, batch,
and print-request state together.

Default state file:

- `/tmp/gscale-zebra/bridge_state.json`

Snapshot sections:

- `scale` - live qty, stable, error, source, port
- `zebra` - last EPC, verify, printer state, RFID status
- `printer` - live connected printer snapshot, kind, label, device paths
- `batch` - active batch state and selected printer mode
- `print_request` - pending or active print job status

Why it exists:

- reduces the need for multiple ad hoc JSON files
- keeps the mobile API, scale worker, and print logic on the same snapshot
- lowers the chance of race conditions by centralizing state updates
