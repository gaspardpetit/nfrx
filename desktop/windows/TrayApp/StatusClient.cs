using System;
using System.Net.Http;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading.Tasks;

namespace TrayApp;

public enum WorkerState
{
    ConnectedIdle,
    ConnectedBusy,
    Connecting,
    Disconnected,
    Draining,
    Terminating,
    Error,
}

public class WorkerStateConverter : JsonConverter<WorkerState>
{
    public override WorkerState Read(ref Utf8JsonReader reader, Type typeToConvert, JsonSerializerOptions options)
    {
        var value = reader.GetString();
        return value switch
        {
            "connected_idle" => WorkerState.ConnectedIdle,
            "connected_busy" => WorkerState.ConnectedBusy,
            "connecting" => WorkerState.Connecting,
            "disconnected" => WorkerState.Disconnected,
            "draining" => WorkerState.Draining,
            "terminating" => WorkerState.Terminating,
            "error" => WorkerState.Error,
            _ => throw new JsonException($"Unknown worker state '{value}'"),
        };
    }

    public override void Write(Utf8JsonWriter writer, WorkerState value, JsonSerializerOptions options)
    {
        var str = value switch
        {
            WorkerState.ConnectedIdle => "connected_idle",
            WorkerState.ConnectedBusy => "connected_busy",
            WorkerState.Connecting => "connecting",
            WorkerState.Disconnected => "disconnected",
            WorkerState.Draining => "draining",
            WorkerState.Terminating => "terminating",
            WorkerState.Error => "error",
            _ => throw new JsonException($"Unknown worker state '{value}'"),
        };
        writer.WriteStringValue(str);
    }
}

public record WorkerStatus(
    [property: JsonPropertyName("state"), JsonConverter(typeof(WorkerStateConverter))]
    WorkerState State,
    [property: JsonPropertyName("connected_to_server")]
    bool ConnectedToServer,
    [property: JsonPropertyName("connected_to_backend")]
    bool ConnectedToBackend,
    [property: JsonPropertyName("current_jobs")]
    int CurrentJobs,
    [property: JsonPropertyName("max_concurrency")]
    int MaxConcurrency,
    [property: JsonPropertyName("models")]
    string[] Models,
    [property: JsonPropertyName("last_error")]
    string LastError,
    [property: JsonPropertyName("last_heartbeat")]
    string LastHeartbeat,
    [property: JsonPropertyName("worker_id")]
    string WorkerId,
    [property: JsonPropertyName("worker_name")]
    string WorkerName,
    [property: JsonPropertyName("version")]
    string Version
);

public class StatusClient
{
    private readonly HttpClient _httpClient;
    private readonly string _url;

    public StatusClient(int port = 4555, HttpClient? httpClient = null)
    {
        _httpClient = httpClient ?? new HttpClient();
        _url = $"http://127.0.0.1:{port}/status";
    }

    public async Task<WorkerStatus> FetchStatusAsync()
    {
        using var stream = await _httpClient.GetStreamAsync(_url);
        var status = await JsonSerializer.DeserializeAsync<WorkerStatus>(stream);
        if (status == null)
        {
            throw new InvalidOperationException("Unable to deserialize worker status");
        }
        return status;
    }
}
