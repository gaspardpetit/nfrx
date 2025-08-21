using System.IO;
using TrayApp;
using Xunit;

public class WorkerConfigTests
{
    [Fact]
    public void RoundTripYaml()
    {
        var cfg = new WorkerConfig
        {
            ServerUrl = "wss://example",
            ClientKey = "secret",
            OllamaBaseUrl = "http://ollama",
            MaxConcurrency = 3,
            StatusPort = 5000
        };
        var path = Path.Combine(Path.GetTempPath(), Path.GetRandomFileName() + ".yaml");
        try
        {
            cfg.Save(path);
            var loaded = WorkerConfig.Load(path);
            Assert.Equal(cfg.ServerUrl, loaded.ServerUrl);
            Assert.Equal(cfg.ClientKey, loaded.ClientKey);
            Assert.Equal(cfg.OllamaBaseUrl, loaded.OllamaBaseUrl);
            Assert.Equal(cfg.MaxConcurrency, loaded.MaxConcurrency);
            Assert.Equal(cfg.StatusPort, loaded.StatusPort);
        }
        finally
        {
            if (File.Exists(path)) File.Delete(path);
        }
    }
}
