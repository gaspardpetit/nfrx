# Windows Tray App Acceptance Test

Use these steps to verify the signed MSI on a clean Windows VM.

1. Install the generated `llamapool.msi`.
2. Confirm the **llamapool** service is installed with *Delayed Auto Start* and running.
3. Launch the tray app from the Start Menu and verify the tooltip shows `Connected Idle` when the worker is up.
4. Send a request through the server; the tray should switch to `Busy` and then back to `Idle` once the job completes.
5. Use **Start Worker** / **Stop Worker** from the tray to control the service and observe status changes.
6. Open **Preferences...**, change a setting such as the status port, save, and restart the service when prompted; the tray should reconnect on the new port.
7. Toggle **Start with Windows**, reboot, and confirm the service follows the selected mode.
8. Choose **Logs...** and **Collect diagnostics** to verify log viewing and the zip output (config, logs, `sc qc llamapool`, `sc query llamapool`).

Successful completion of the above steps validates the end-to-end Windows integration.
