using System;
using System.IO;

namespace TrayApp;

/// <summary>
/// Well-known locations for the Windows worker service.
/// </summary>
public static class Paths
{
    public static readonly string ProgramDataDir =
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "infero");

    public static readonly string LogsDir = Path.Combine(ProgramDataDir, "Logs");

    public static readonly string ConfigPath = Path.Combine(ProgramDataDir, "worker.yaml");

    public static readonly string TokenPath = Path.Combine(ProgramDataDir, "token");

    public static readonly string LogPath = Path.Combine(LogsDir, "worker.log");

    public static readonly string BinaryPath = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles),
        "infero",
        "infero-llm.exe");
}

