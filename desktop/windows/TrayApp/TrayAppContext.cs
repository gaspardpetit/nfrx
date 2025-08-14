using System;
using System.Drawing;
using System.Net.Http;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace TrayApp;

public class TrayAppContext : ApplicationContext
{
    private readonly NotifyIcon _notifyIcon;
    private readonly ToolStripMenuItem _statusItem;
    private readonly ToolStripMenuItem _detailsItem;
    private readonly ToolStripMenuItem _startStopItem;
    private readonly ToolStripMenuItem _startWithWindowsItem;

    private readonly StatusClient _statusClient;
    private readonly Timer _statusTimer;
    private WorkerStatus? _currentStatus;
    private string? _lastError;

    public TrayAppContext()
    {
        _statusItem = new ToolStripMenuItem("Status: Unknown")
        {
            Enabled = false
        };

        _detailsItem = new ToolStripMenuItem("Details...", null, OnDetailsClicked)
        {
            Enabled = false
        };

        _startStopItem = new ToolStripMenuItem("Start Worker", null, OnStartStopClicked);

        _startWithWindowsItem = new ToolStripMenuItem("Start with Windows")
        {
            CheckOnClick = true
        };
        _startWithWindowsItem.Click += OnStartWithWindowsClicked;

        var contextMenu = new ContextMenuStrip();
        contextMenu.Items.AddRange(new ToolStripItem[]
        {
            _statusItem,
            _detailsItem,
            new ToolStripSeparator(),
            _startStopItem,
            new ToolStripMenuItem("Preferences...", null, OnPreferencesClicked),
            new ToolStripMenuItem("Logs...", null, OnLogsClicked),
            _startWithWindowsItem,
            new ToolStripMenuItem("Check for Updates", null, OnCheckForUpdatesClicked),
            new ToolStripMenuItem("Exit", null, OnExitClicked)
        });

        _notifyIcon = new NotifyIcon
        {
            Icon = SystemIcons.Application,
            ContextMenuStrip = contextMenu,
            Visible = true,
            Text = "llamapool"
        };

        _statusClient = new StatusClient();
        _statusTimer = new Timer { Interval = 2000 };
        _statusTimer.Tick += async (_, _) => await RefreshStatusAsync();
        _statusTimer.Start();
    }

    private void OnStartStopClicked(object? sender, EventArgs e)
    {
        // TODO: Start or stop the worker service
        MessageBox.Show("Start/Stop Worker clicked");
    }

    private void OnPreferencesClicked(object? sender, EventArgs e)
    {
        // TODO: Open preferences dialog
        MessageBox.Show("Preferences clicked");
    }

    private void OnLogsClicked(object? sender, EventArgs e)
    {
        // TODO: Open logs viewer
        MessageBox.Show("Logs clicked");
    }

    private void OnStartWithWindowsClicked(object? sender, EventArgs e)
    {
        // TODO: Toggle start with Windows
        MessageBox.Show("Start with Windows toggled");
    }

    private void OnCheckForUpdatesClicked(object? sender, EventArgs e)
    {
        // TODO: Check for application updates
        MessageBox.Show("Check for Updates clicked");
    }

    private void OnExitClicked(object? sender, EventArgs e)
    {
        _statusTimer.Stop();
        _notifyIcon.Visible = false;
        Application.Exit();
    }

    private async Task RefreshStatusAsync()
    {
        try
        {
            var status = await _statusClient.FetchStatusAsync();
            _currentStatus = status;
            _lastError = string.IsNullOrEmpty(status.LastError) ? null : status.LastError;

            var text = TextForState(status.State);
            _statusItem.Text = $"Status: {text}";
            _notifyIcon.Text = $"llamapool - {text}";
            _detailsItem.Enabled = true;
        }
        catch (HttpRequestException ex)
        {
            _currentStatus = null;
            _lastError = ex.Message;
            _statusItem.Text = "Status: Disconnected";
            _notifyIcon.Text = "llamapool - Disconnected";
            _detailsItem.Enabled = !string.IsNullOrEmpty(_lastError);
        }
        catch (Exception ex)
        {
            _currentStatus = null;
            _lastError = ex.Message;
            _statusItem.Text = "Status: Error";
            _notifyIcon.Text = "llamapool - Error";
            _detailsItem.Enabled = true;
        }
    }

    private void OnDetailsClicked(object? sender, EventArgs e)
    {
        if (_currentStatus != null)
        {
            var s = _currentStatus;
            var msg = $"Worker: {s.WorkerName} ({s.WorkerId})\n" +
                      $"Version: {s.Version}\n" +
                      $"Connected to server: {s.ConnectedToServer}\n" +
                      $"Connected to Ollama: {s.ConnectedToOllama}\n" +
                      $"Jobs: {s.CurrentJobs}/{s.MaxConcurrency}\n" +
                      $"Last error: {(string.IsNullOrEmpty(s.LastError) ? "<none>" : s.LastError)}";
            MessageBox.Show(msg, "Worker Details");
        }
        else if (!string.IsNullOrEmpty(_lastError))
        {
            MessageBox.Show(_lastError, "Worker Error");
        }
    }

    private static string TextForState(WorkerState state) => state switch
    {
        WorkerState.ConnectedIdle => "Connected",
        WorkerState.ConnectedBusy => "Busy",
        WorkerState.Connecting => "Connecting",
        WorkerState.Disconnected => "Disconnected",
        WorkerState.Draining => "Draining",
        WorkerState.Terminating => "Terminating",
        WorkerState.Error => "Error",
        _ => "Unknown"
    };
}
