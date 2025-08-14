using System.IO;
using YamlDotNet.Serialization;
using YamlDotNet.Serialization.NamingConventions;

namespace TrayApp;

public class WorkerConfig
{
    public string ServerUrl { get; set; } = "";
    public string WorkerKey { get; set; } = "";
    public string OllamaBaseUrl { get; set; } = "http://127.0.0.1:11434";
    public int MaxConcurrency { get; set; } = 2;

    public string StatusAddr
    {
        get => $"127.0.0.1:{StatusPort}";
        set
        {
            var parts = value?.Split(':');
            if (parts?.Length > 1 && int.TryParse(parts[^1], out var port))
            {
                StatusPort = port;
            }
        }
    }

    [YamlIgnore]
    public int StatusPort { get; set; } = 4555;

    public static WorkerConfig Load(string path)
    {
        if (!File.Exists(path))
        {
            return new WorkerConfig();
        }

        var yaml = File.ReadAllText(path);
        var deserializer = new DeserializerBuilder()
            .WithNamingConvention(UnderscoredNamingConvention.Instance)
            .IgnoreUnmatchedProperties()
            .Build();
        var cfg = deserializer.Deserialize<WorkerConfig>(yaml);
        return cfg ?? new WorkerConfig();
    }

    public void Save(string path)
    {
        var dir = Path.GetDirectoryName(path);
        if (!string.IsNullOrEmpty(dir))
        {
            Directory.CreateDirectory(dir);
        }
        var serializer = new SerializerBuilder()
            .WithNamingConvention(UnderscoredNamingConvention.Instance)
            .Build();
        var yaml = serializer.Serialize(this);
        File.WriteAllText(path, yaml);
    }
}

