using System.Diagnostics;
using System.IO.Compression;
using System.Linq;
using System.Net.Http;
using System.Net.Http.Json;
using System.ServiceProcess;
using System.Text.Json.Serialization;
using System.Threading.Tasks;
using System.Windows.Forms;
using Timer = System.Windows.Forms.Timer;

namespace TrayApp;

public class TrayAppContext : ApplicationContext
{
    private readonly NotifyIcon _notifyIcon;
    private readonly ToolStripMenuItem _statusItem;
    private readonly ToolStripMenuItem _versionItem;
    private readonly ToolStripMenuItem _detailsItem;
    private readonly ToolStripMenuItem _startStopItem;
    private readonly ToolStripMenuItem _startWithWindowsItem;
    private readonly ToolStripMenuItem _drainItem;
    private readonly ToolStripMenuItem _undrainItem;
    private readonly ToolStripMenuItem _shutdownItem;

    private StatusClient _statusClient;
    private ControlClient _controlClient;
    private readonly Timer _statusTimer;
    private readonly Timer _updateTimer;
    private readonly HttpClient _updateClient = new();
    private WorkerStatus? _currentStatus;
    private string? _currentVersion;
    private string? _latestVersion;
    private string? _notifiedVersion;
    private string? _lastError;
    private WorkerConfig _config;
    private bool _controlsAvailable;
    private const string ReleaseApi = "https://api.github.com/repos/gaspardpetit/llamapool/releases/latest";
    private const string ReleasePage = "https://github.com/gaspardpetit/llamapool/releases";

    private const string ServiceName = "llamapool";

    public TrayAppContext()
    {
        _statusItem = new ToolStripMenuItem("Status: Unknown")
        {
            Enabled = false
        };

        _versionItem = new ToolStripMenuItem("Version: Unknown")
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

        _drainItem = new ToolStripMenuItem("Drain", null, OnDrainClicked)
        {
            Enabled = false
        };
        _undrainItem = new ToolStripMenuItem("Undrain", null, OnUndrainClicked)
        {
            Enabled = false
        };
        _shutdownItem = new ToolStripMenuItem("Shutdown after drain", null, OnShutdownClicked)
        {
            Enabled = false
        };

        var contextMenu = new ContextMenuStrip();
        contextMenu.Items.AddRange(new ToolStripItem[]
        {
            _statusItem,
            _versionItem,
            _detailsItem,
            new ToolStripSeparator(),
            _startStopItem,
            _drainItem,
            _undrainItem,
            _shutdownItem,
            new ToolStripMenuItem("Preferences...", null, OnPreferencesClicked),
            new ToolStripMenuItem("Logs...", null, OnLogsClicked),
            new ToolStripMenuItem("Open Config Folder", null, OnOpenConfigFolderClicked),
            new ToolStripMenuItem("Open Logs Folder", null, OnOpenLogsFolderClicked),
            _startWithWindowsItem,
            new ToolStripMenuItem("Collect Diagnostics", null, OnCollectDiagnosticsClicked),
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

        _config = WorkerConfig.Load(Paths.ConfigPath);
        _statusClient = new StatusClient(_config.StatusPort);
        _controlClient = new ControlClient(_config.StatusPort);
        _statusTimer = new System.Windows.Forms.Timer { Interval = 2000 };
        _statusTimer.Tick += async (_, _) => await RefreshStatusAsync();
        _statusTimer.Start();

        _updateTimer = new System.Windows.Forms.Timer { Interval = 24 * 60 * 60 * 1000 };
        _updateTimer.Tick += async (_, _) => await CheckForUpdatesAsync();
        _updateTimer.Start();
        _updateClient.DefaultRequestHeaders.UserAgent.ParseAdd("llamapool-trayapp");

        _controlsAvailable = false;
        _ = ProbeControlEndpointsAsync();

        RefreshServiceState();
        _ = CheckForUpdatesAsync();
    }

    private void OnStartStopClicked(object? sender, EventArgs e)
    {
        try
        {
            using var sc = new ServiceController(ServiceName);
            if (sc.Status is ServiceControllerStatus.Running or ServiceControllerStatus.StartPending)
            {
                sc.Stop();
                sc.WaitForStatus(ServiceControllerStatus.Stopped, TimeSpan.FromSeconds(10));
            }
            else if (sc.Status is ServiceControllerStatus.Stopped or ServiceControllerStatus.StopPending)
            {
                sc.Start();
                sc.WaitForStatus(ServiceControllerStatus.Running, TimeSpan.FromSeconds(10));
            }
        }
        catch (Exception ex)
        {
            MessageBox.Show($"Failed to control service: {ex.Message}");
        }
        finally
        {
            RefreshServiceState();
        }
    }

    private void OnPreferencesClicked(object? sender, EventArgs e)
    {
        using var form = new PreferencesForm(new WorkerConfig
        {
            ServerUrl = _config.ServerUrl,
            WorkerKey = _config.WorkerKey,
            OllamaBaseUrl = _config.OllamaBaseUrl,
            MaxConcurrency = _config.MaxConcurrency,
            StatusPort = _config.StatusPort
        });
        if (form.ShowDialog() == DialogResult.OK)
        {
            var running = IsServiceRunning();
            form.Config.Save(Paths.ConfigPath);
            _config = form.Config;
            _statusClient = new StatusClient(_config.StatusPort);
            _controlClient = new ControlClient(_config.StatusPort);
            _controlsAvailable = false;
            _ = ProbeControlEndpointsAsync();
            if (running)
            {
                var res = MessageBox.Show("Restart worker service now?", "llamapool", MessageBoxButtons.YesNo);
                if (res == DialogResult.Yes)
                {
                    try
                    {
                        using var sc = new ServiceController(ServiceName);
                        sc.Stop();
                        sc.WaitForStatus(ServiceControllerStatus.Stopped, TimeSpan.FromSeconds(10));
                        sc.Start();
                        sc.WaitForStatus(ServiceControllerStatus.Running, TimeSpan.FromSeconds(10));
                    }
                    catch (Exception ex)
                    {
                        MessageBox.Show($"Failed to restart service: {ex.Message}");
                    }
                }
            }
        }
    }

    private void OnOpenConfigFolderClicked(object? sender, EventArgs e)
    {
        try
        {
            Directory.CreateDirectory(Paths.ProgramDataDir);
            Process.Start("explorer.exe", Paths.ProgramDataDir);
        }
        catch (Exception ex)
        {
            MessageBox.Show($"Failed to open config folder: {ex.Message}");
        }
    }

    private void OnOpenLogsFolderClicked(object? sender, EventArgs e)
    {
        try
        {
            Directory.CreateDirectory(Paths.LogsDir);
            Process.Start("explorer.exe", Paths.LogsDir);
        }
        catch (Exception ex)
        {
            MessageBox.Show($"Failed to open logs folder: {ex.Message}");
        }
    }

    private void OnLogsClicked(object? sender, EventArgs e)
    {
        try
        {
            using var form = new LogsForm(Paths.LogPath);
            form.ShowDialog();
        }
        catch (Exception ex)
        {
            MessageBox.Show($"Failed to open logs: {ex.Message}");
        }
    }

    private void OnCollectDiagnosticsClicked(object? sender, EventArgs e)
    {
        try
        {
            var tempDir = Path.Combine(Path.GetTempPath(), $"llamapool-diagnostics-{Guid.NewGuid()}");
            Directory.CreateDirectory(tempDir);

            if (File.Exists(Paths.ConfigPath))
            {
                File.Copy(Paths.ConfigPath, Path.Combine(tempDir, "worker.yaml"), true);
            }

            if (File.Exists(Paths.LogPath))
            {
                Directory.CreateDirectory(Path.Combine(tempDir, "Logs"));
                File.Copy(Paths.LogPath, Path.Combine(tempDir, "Logs", "worker.log"), true);
            }

            File.WriteAllText(Path.Combine(tempDir, "sc_qc.txt"), RunProcessCapture("sc.exe", $"qc {ServiceName}"));
            File.WriteAllText(Path.Combine(tempDir, "sc_query.txt"), RunProcessCapture("sc.exe", $"query {ServiceName}"));

            var desktop = Environment.GetFolderPath(Environment.SpecialFolder.DesktopDirectory);
            var zipPath = Path.Combine(desktop, $"llamapool-diagnostics-{DateTime.Now:yyyyMMddHHmmss}.zip");
            ZipFile.CreateFromDirectory(tempDir, zipPath);

            MessageBox.Show($"Diagnostics collected to {zipPath}");
        }
        catch (Exception ex)
        {
            MessageBox.Show($"Failed to collect diagnostics: {ex.Message}");
        }
    }

    private static string RunProcessCapture(string fileName, string arguments)
    {
        try
        {
            var psi = new ProcessStartInfo(fileName, arguments)
            {
                RedirectStandardOutput = true,
                UseShellExecute = false,
                CreateNoWindow = true
            };
            using var proc = Process.Start(psi);
            if (proc == null) return string.Empty;
            var output = proc.StandardOutput.ReadToEnd();
            proc.WaitForExit();
            return output;
        }
        catch
        {
            return string.Empty;
        }
    }

    private async void OnDrainClicked(object? sender, EventArgs e)
    {
        await SendControlAsync("drain");
    }

    private async void OnUndrainClicked(object? sender, EventArgs e)
    {
        await SendControlAsync("undrain");
    }

    private async void OnShutdownClicked(object? sender, EventArgs e)
    {
        await SendControlAsync("shutdown");
    }

    private async Task SendControlAsync(string command)
    {
        try
        {
            await _controlClient.SendCommandAsync(command);
        }
        catch (Exception ex)
        {
            MessageBox.Show($"Failed to send '{command}': {ex.Message}");
        }
        finally
        {
            await RefreshStatusAsync();
        }
    }

    private void OnStartWithWindowsClicked(object? sender, EventArgs e)
    {
        try
        {
            SetServiceStartMode(_startWithWindowsItem.Checked);
        }
        catch (Exception ex)
        {
            MessageBox.Show($"Failed to update start mode: {ex.Message}");
        }
        finally
        {
            RefreshServiceState();
        }
    }

    private async void OnCheckForUpdatesClicked(object? sender, EventArgs e)
    {
        await CheckForUpdatesAsync(true);
    }

    private async Task CheckForUpdatesAsync(bool userInitiated = false)
    {
        try
        {
            var rel = await _updateClient.GetFromJsonAsync<GithubRelease>(ReleaseApi);
            if (rel == null || string.IsNullOrEmpty(rel.TagName))
            {
                return;
            }

            _latestVersion = rel.TagName;
            UpdateVersionItem();

            if (_currentVersion == null)
            {
                try
                {
                    var status = await _statusClient.FetchStatusAsync();
                    _currentStatus = status;
                    _currentVersion = status.Version;
                }
                catch
                {
                    // ignore
                }
            }

            var current = _currentVersion;
            if (!string.IsNullOrEmpty(current) && _latestVersion != current && _latestVersion != _notifiedVersion)
            {
                _notifiedVersion = _latestVersion;
                if (userInitiated)
                {
                    var res = MessageBox.Show($"Version {_latestVersion} is available. Open release page?", "Update", MessageBoxButtons.YesNo, MessageBoxIcon.Information);
                    if (res == DialogResult.Yes)
                    {
                        Process.Start("explorer.exe", ReleasePage);
                    }
                }
                else
                {
                    _notifyIcon.BalloonTipTitle = "llamapool";
                    _notifyIcon.BalloonTipText = $"Version {_latestVersion} is available (current {current}).";
                    EventHandler? handler = null;
                    handler = (_, _) =>
                    {
                        Process.Start("explorer.exe", ReleasePage);
                        _notifyIcon.BalloonTipClicked -= handler;
                    };
                    _notifyIcon.BalloonTipClicked += handler;
                    _notifyIcon.ShowBalloonTip(10000);
                }
            }
            else if (userInitiated)
            {
                MessageBox.Show("No updates available.", "Update", MessageBoxButtons.OK, MessageBoxIcon.Information);
            }
        }
        catch (Exception ex)
        {
            if (userInitiated)
            {
                MessageBox.Show($"Failed to check for updates: {ex.Message}", "Update", MessageBoxButtons.OK, MessageBoxIcon.Error);
            }
        }
    }

    private void UpdateVersionItem()
    {
        var current = _currentVersion ?? "unknown";
        if (!string.IsNullOrEmpty(_latestVersion) && _latestVersion != current)
        {
            _versionItem.Text = $"Version: {current} (latest {_latestVersion})";
        }
        else
        {
            _versionItem.Text = $"Version: {current}";
        }
    }

    private record GithubRelease([property: JsonPropertyName("tag_name")] string TagName);

    private void OnExitClicked(object? sender, EventArgs e)
    {
        _statusTimer.Stop();
        _notifyIcon.Visible = false;
        Application.Exit();
    }

    private async Task RefreshStatusAsync()
    {
        RefreshServiceState();

        try
        {
            var status = await _statusClient.FetchStatusAsync();
            _currentStatus = status;
            _currentVersion = status.Version;
            _lastError = string.IsNullOrEmpty(status.LastError) ? null : status.LastError;

            var text = TextForState(status.State);
            _statusItem.Text = $"Status: {text}";
            _notifyIcon.Text = $"llamapool - {text}";
            _detailsItem.Enabled = true;
            UpdateVersionItem();
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

        UpdateControlMenuItems();
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

    private void UpdateControlMenuItems()
    {
        if (!_controlsAvailable)
        {
            _drainItem.Enabled = false;
            _undrainItem.Enabled = false;
            _shutdownItem.Enabled = false;
            return;
        }

        var state = _currentStatus?.State;
        _drainItem.Enabled = state != null && state != WorkerState.Draining && state != WorkerState.Terminating;
        _undrainItem.Enabled = state == WorkerState.Draining;
        _shutdownItem.Enabled = state != null && state != WorkerState.Terminating;
    }

    private async Task ProbeControlEndpointsAsync()
    {
        _controlsAvailable = await _controlClient.ProbeAsync();
        UpdateControlMenuItems();
    }

    private void RefreshServiceState()
    {
        try
        {
            using var sc = new ServiceController(ServiceName);
            _startStopItem.Enabled = sc.Status is not (ServiceControllerStatus.StartPending or ServiceControllerStatus.StopPending);
            _startStopItem.Text = sc.Status is ServiceControllerStatus.Running or ServiceControllerStatus.StartPending
                ? "Stop Worker"
                : "Start Worker";
            _startWithWindowsItem.Enabled = true;
            _startWithWindowsItem.Checked = IsServiceAutoStart();
        }
        catch
        {
            _startStopItem.Enabled = false;
            _startStopItem.Text = "Start Worker";
            _startWithWindowsItem.Enabled = false;
            _startWithWindowsItem.Checked = false;
        }
    }

    private static bool IsServiceAutoStart()
    {
        try
        {
            var psi = new ProcessStartInfo("sc.exe", $"qc {ServiceName}")
            {
                RedirectStandardOutput = true,
                UseShellExecute = false,
                CreateNoWindow = true
            };
            using var proc = Process.Start(psi);
            if (proc == null) return false;
            var output = proc.StandardOutput.ReadToEnd();
            proc.WaitForExit();
            return output.Contains("AUTO_START", StringComparison.OrdinalIgnoreCase);
        }
        catch
        {
            return false;
        }
    }

    private static void SetServiceStartMode(bool auto)
    {
        var startType = auto ? "delayed-auto" : "demand";
        var psi = new ProcessStartInfo("sc.exe", $"config {ServiceName} start= {startType}")
        {
            UseShellExecute = false,
            CreateNoWindow = true
        };
        using var proc = Process.Start(psi);
        proc?.WaitForExit();
    }

    private static bool IsServiceRunning()
    {
        try
        {
            using var sc = new ServiceController(ServiceName);
            return sc.Status == ServiceControllerStatus.Running;
        }
        catch
        {
            return false;
        }
    }
}
