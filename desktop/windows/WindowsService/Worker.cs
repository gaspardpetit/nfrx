using System.Diagnostics;
using System.IO;

namespace WindowsService;

public class Worker : BackgroundService
{
    private readonly ILogger<Worker> _logger;
    private Process? _process;

    public Worker(ILogger<Worker> logger)
    {
        _logger = logger;
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var programFiles = Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles);
        var workerExe = Path.Combine(programFiles, "llamapool", "llamapool-worker.exe");

        var programData = Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData);
        var dataDir = Path.Combine(programData, "llamapool");
        Directory.CreateDirectory(dataDir);

        var logsDir = Path.Combine(dataDir, "Logs");
        Directory.CreateDirectory(logsDir);

        var configPath = Path.Combine(dataDir, "worker.yaml");
        var logPath = Path.Combine(logsDir, "worker.log");

        var psi = new ProcessStartInfo
        {
            FileName = workerExe,
            Arguments = $"--status-addr 127.0.0.1:4555 --config \"{configPath}\"",
            WorkingDirectory = dataDir,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            UseShellExecute = false,
            CreateNoWindow = true
        };

        _process = new Process { StartInfo = psi, EnableRaisingEvents = true };

        try
        {
            _process.Start();

            using var logWriter = TextWriter.Synchronized(new StreamWriter(logPath, append: true));
            _process.OutputDataReceived += (_, e) => { if (e.Data != null) logWriter.WriteLine(e.Data); };
            _process.ErrorDataReceived += (_, e) => { if (e.Data != null) logWriter.WriteLine(e.Data); };
            _process.BeginOutputReadLine();
            _process.BeginErrorReadLine();

            await _process.WaitForExitAsync(stoppingToken);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to run worker process");
        }
    }

    public override Task StopAsync(CancellationToken cancellationToken)
    {
        if (_process != null && !_process.HasExited)
        {
            try
            {
                _process.CloseMainWindow();
                if (!_process.WaitForExit(5000))
                {
                    _process.Kill();
                }
            }
            catch (Exception ex)
            {
                _logger.LogWarning(ex, "Failed to stop worker process gracefully");
            }
        }

        return base.StopAsync(cancellationToken);
    }
}
