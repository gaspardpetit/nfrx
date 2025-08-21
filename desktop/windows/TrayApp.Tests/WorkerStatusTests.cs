using System.Text.Json;
using TrayApp;
using Xunit;

public class WorkerStatusTests
{
    private const string SampleJson = """
    {
      "state": "connected_idle",
      "connected_to_server": true,
      "connected_to_backend": true,
      "current_jobs": 1,
      "max_concurrency": 2,
      "models": ["llama3"],
      "last_error": "",
      "last_heartbeat": "2024-05-01T12:00:00Z",
      "worker_id": "1234",
      "worker_name": "test-worker",
      "version": "v0.0.1"
    }
    """;

    [Fact]
    public void DeserializeSample()
    {
        var status = JsonSerializer.Deserialize<WorkerStatus>(SampleJson);
        Assert.NotNull(status);
        Assert.Equal(WorkerState.ConnectedIdle, status!.State);
        Assert.Equal("test-worker", status.WorkerName);
        Assert.Equal(1, status.CurrentJobs);
    }
}
