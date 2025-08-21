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
            CompletionBaseUrl = "http://ollama/v1",
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
            Assert.Equal(cfg.CompletionBaseUrl, loaded.CompletionBaseUrl);
            Assert.Equal(cfg.MaxConcurrency, loaded.MaxConcurrency);
            Assert.Equal(cfg.StatusPort, loaded.StatusPort);
        }
        finally
        {
            if (File.Exists(path)) File.Delete(path);
        }
    }
}
