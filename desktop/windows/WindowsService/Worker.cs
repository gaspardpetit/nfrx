using System.Diagnostics;
using System.IO;
using System.Linq;
using Microsoft.Extensions.Hosting;

namespace WindowsService;

public class Worker : BackgroundService
{
    private readonly ILogger<Worker> _logger;
    private readonly IHostApplicationLifetime _lifetime;
    private Process? _process;
    private JobObject? _job;

    public Worker(ILogger<Worker> logger, IHostApplicationLifetime lifetime)
    {
        _logger = logger;
        _lifetime = lifetime;
        AppDomain.CurrentDomain.ProcessExit += (_, _) => TryKillWorker();
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var programFiles = Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles);
        var workerExe = Path.Combine(programFiles, "infx", "infx-llm.exe");

        var programData = Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData);
        var dataDir = Path.Combine(programData, "infx");
        Directory.CreateDirectory(dataDir);

        var logsDir = Path.Combine(dataDir, "Logs");
        Directory.CreateDirectory(logsDir);

        var configPath = Path.Combine(dataDir, "worker.yaml");
        var logPath = Path.Combine(logsDir, "worker.log");

        if (Process.GetProcessesByName(Path.GetFileNameWithoutExtension(workerExe)).Any())
        {
            _logger.LogWarning("infx-llm is already running; exiting service");
            _lifetime.StopApplication();
            return;
        }

        var psi = new ProcessStartInfo
        {
            FileName = workerExe,
            Arguments = $"--status-addr 127.0.0.1:4555 --config \"{configPath}\" --reconnect",
            WorkingDirectory = dataDir,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            UseShellExecute = false,
            CreateNoWindow = true
        };

        try
        {
            _job = new JobObject();
        }
        catch (Exception ex)
        {
            _logger.LogWarning(ex, "Failed to create job object; worker may survive service termination");
        }

        _process = new Process { StartInfo = psi, EnableRaisingEvents = true };
        _process.Exited += (_, _) =>
        {
            if (!stoppingToken.IsCancellationRequested)
            {
                _logger.LogWarning("worker process exited with code {ExitCode}", _process?.ExitCode);
                _lifetime.StopApplication();
            }
        };

        try
        {
            _process.Start();

            try
            {
                _job?.AddProcess(_process.Handle);
            }
            catch (Exception ex)
            {
                _logger.LogWarning(ex, "Failed to assign worker to job object");
            }

            using var logWriter = TextWriter.Synchronized(new StreamWriter(logPath, append: true));
            _process.OutputDataReceived += (_, e) => { if (e.Data != null) logWriter.WriteLine(e.Data); };
            _process.ErrorDataReceived += (_, e) => { if (e.Data != null) logWriter.WriteLine(e.Data); };
            _process.BeginOutputReadLine();
            _process.BeginErrorReadLine();

            await _process.WaitForExitAsync(stoppingToken);

            if (!stoppingToken.IsCancellationRequested)
            {
                _lifetime.StopApplication();
            }
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to run worker process");
            _lifetime.StopApplication();
        }
    }

    public override Task StopAsync(CancellationToken cancellationToken)
    {
        TryKillWorker();
        return base.StopAsync(cancellationToken);
    }

    private void TryKillWorker()
    {
        try
        {
            if (_process != null && !_process.HasExited)
            {
                _process.CloseMainWindow();
                if (!_process.WaitForExit(5000))
                {
                    _process.Kill(entireProcessTree: true);
                }
            }
        }
        catch (Exception ex)
        {
            _logger.LogWarning(ex, "Failed to stop worker process gracefully");
        }
        finally
        {
            _job?.Dispose();
            _job = null;
        }
    }
}
