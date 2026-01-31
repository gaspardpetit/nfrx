using System;
using System.Collections.Generic;
using System.IO;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading;
using System.Threading.Tasks;

namespace NfrxSdk;

public sealed class NfrxJobsClient
{
    private readonly string _baseUrl;
    private readonly string? _apiKey;
    private readonly HttpClient _http;

    public NfrxJobsClient(string baseUrl, string? apiKey, HttpClient httpClient)
    {
        _baseUrl = baseUrl.TrimEnd('/');
        _apiKey = apiKey;
        _http = httpClient;
    }

    public async Task<JobCreateResponse> CreateJobAsync(string jobType, Dictionary<string, object>? metadata)
    {
        var payload = new Dictionary<string, object>
        {
            { "type", jobType },
            { "metadata", metadata ?? new Dictionary<string, object>() }
        };
        using var req = new HttpRequestMessage(HttpMethod.Post, _baseUrl + "/api/jobs");
        AddAuth(req);
        req.Content = new StringContent(JsonSerializer.Serialize(payload), Encoding.UTF8, "application/json");
        using var resp = await _http.SendAsync(req).ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync().ConfigureAwait(false);
        return JsonSerializer.Deserialize<JobCreateResponse>(json, JsonOptions.Default)
               ?? throw new InvalidOperationException("Invalid create job response");
    }

    public async Task<Dictionary<string, JsonElement>> GetJobAsync(string jobId)
    {
        using var req = new HttpRequestMessage(HttpMethod.Get, _baseUrl + $"/api/jobs/{jobId}");
        AddAuth(req);
        using var resp = await _http.SendAsync(req).ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync().ConfigureAwait(false);
        return DeserializeData(json);
    }

    public async Task CancelJobAsync(string jobId)
    {
        using var req = new HttpRequestMessage(HttpMethod.Post, _baseUrl + $"/api/jobs/{jobId}/cancel");
        AddAuth(req);
        using var resp = await _http.SendAsync(req).ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();
    }

    public async IAsyncEnumerable<(string Type, Dictionary<string, JsonElement> Data)> StreamEventsAsync(
        string jobId,
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellationToken)
    {
        using var req = new HttpRequestMessage(HttpMethod.Get, _baseUrl + $"/api/jobs/{jobId}/events");
        AddAuth(req);
        req.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("text/event-stream"));
        using var resp = await _http.SendAsync(req, HttpCompletionOption.ResponseHeadersRead, cancellationToken)
            .ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();

        await using var stream = await resp.Content.ReadAsStreamAsync(cancellationToken).ConfigureAwait(false);
        using var reader = new StreamReader(stream);

        string? eventType = null;
        var dataLines = new List<string>();

        while (!reader.EndOfStream && !cancellationToken.IsCancellationRequested)
        {
            var line = await reader.ReadLineAsync().ConfigureAwait(false);
            if (line == null)
            {
                break;
            }
            if (line.Length == 0)
            {
                if (dataLines.Count > 0)
                {
                    var data = string.Join("\n", dataLines);
                    var payload = DeserializeData(data);
                    yield return (eventType ?? "message", payload);
                }
                eventType = null;
                dataLines.Clear();
                continue;
            }
            if (line.StartsWith(":", StringComparison.Ordinal))
            {
                continue;
            }
            if (line.StartsWith("event:", StringComparison.OrdinalIgnoreCase))
            {
                eventType = line.Substring(6).Trim();
                continue;
            }
            if (line.StartsWith("data:", StringComparison.OrdinalIgnoreCase))
            {
                dataLines.Add(line.Substring(5).TrimStart());
            }
        }
    }

    private void AddAuth(HttpRequestMessage req)
    {
        if (!string.IsNullOrWhiteSpace(_apiKey))
        {
            req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", _apiKey);
        }
    }

    internal static Dictionary<string, JsonElement> DeserializeData(string data)
    {
        try
        {
            return JsonSerializer.Deserialize<Dictionary<string, JsonElement>>(data, JsonOptions.Default)
                   ?? new Dictionary<string, JsonElement>();
        }
        catch
        {
            return new Dictionary<string, JsonElement>
            {
                { "raw", JsonDocument.Parse("\"" + data + "\"").RootElement }
            };
        }
    }
}

public sealed class NfrxJobsRunner : IDisposable
{
    private readonly NfrxJobsClient _jobs;
    private readonly NfrxTransferClient _transfer;
    private readonly HttpClient _http;
    private readonly bool _ownsHttp;

    public NfrxJobsRunner(string baseUrl, string? apiKey, int timeoutSeconds, HttpClient? httpClient = null)
    {
        _http = httpClient ?? new HttpClient();
        _ownsHttp = httpClient == null;
        _jobs = new NfrxJobsClient(baseUrl, apiKey, _http);
        _transfer = new NfrxTransferClient(baseUrl, apiKey, _http);
        TimeoutSeconds = timeoutSeconds;
    }

    public int TimeoutSeconds { get; set; }

    public async Task<JobSession> CreateJobSessionAsync(
        string jobType,
        Dictionary<string, object>? metadata,
        PayloadProvider payloadProvider,
        ResultConsumer resultConsumer)
    {
        var created = await _jobs.CreateJobAsync(jobType, metadata);
        return new JobSession(created.JobId, payloadProvider, resultConsumer);
    }

    public async Task RunSessionAsync(JobSession session, StatusHandler? onStatus, int? timeoutSeconds = null)
    {
        var timeout = timeoutSeconds ?? TimeoutSeconds;
        using var cts = new CancellationTokenSource(TimeSpan.FromSeconds(timeout));
        await foreach (var (type, data) in _jobs.StreamEventsAsync(session.JobId, cts.Token))
        {
            if (type == "status" && onStatus != null)
            {
                await onStatus(data);
            }
            if (type == "payload")
            {
                await HandlePayload(session, data);
            }
            if (type == "result")
            {
                await HandleResult(session, data);
            }
            if (type == "status" && IsTerminal(data))
            {
                return;
            }
        }
    }

    public async Task<Dictionary<string, JsonElement>> GetJobAsync(string jobId)
    {
        return await _jobs.GetJobAsync(jobId);
    }

    private async Task HandlePayload(JobSession session, Dictionary<string, JsonElement> payload)
    {
        var key = payload.TryGetValue("key", out var k) ? k.GetString() ?? "payload" : "payload";
        var provided = await session.PayloadProvider(key, payload);
        if (provided == null)
        {
            return;
        }
        var (data, contentType) = provided.Value;
        var channel = GetChannel(payload);
        await _transfer.UploadAsync(channel, data, contentType);
    }

    private async Task HandleResult(JobSession session, Dictionary<string, JsonElement> payload)
    {
        var key = payload.TryGetValue("key", out var k) ? k.GetString() ?? "result" : "result";
        var channel = GetChannel(payload);
        var data = await _transfer.DownloadAsync(channel);
        await session.ResultConsumer(key, data, payload);
    }

    private static string GetChannel(Dictionary<string, JsonElement> payload)
    {
        if (payload.TryGetValue("channel_id", out var id))
        {
            var channelId = id.GetString();
            if (!string.IsNullOrWhiteSpace(channelId))
            {
                return channelId;
            }
        }
        if (payload.TryGetValue("url", out var url))
        {
            var channelUrl = url.GetString();
            if (!string.IsNullOrWhiteSpace(channelUrl))
            {
                return channelUrl;
            }
        }
        throw new InvalidOperationException("Missing transfer channel info");
    }

    private static bool IsTerminal(Dictionary<string, JsonElement> payload)
    {
        if (!payload.TryGetValue("status", out var status))
        {
            return false;
        }
        var value = status.GetString();
        return value == "completed" || value == "failed" || value == "canceled";
    }

    public void Dispose()
    {
        if (_ownsHttp)
        {
            _http.Dispose();
        }
    }
}

public sealed class NfrxJobsWorker : IDisposable
{
    private readonly string _baseUrl;
    private readonly string? _clientKey;
    private readonly HttpClient _http;
    private readonly bool _ownsHttp;

    public NfrxJobsWorker(string baseUrl, string? clientKey, HttpClient? httpClient = null)
    {
        _baseUrl = baseUrl.TrimEnd('/');
        _clientKey = clientKey;
        _http = httpClient ?? new HttpClient();
        _ownsHttp = httpClient == null;
    }

    public async Task<JobClaimResponse?> ClaimJobAsync(
        List<string>? types,
        int maxWaitSeconds,
        CancellationToken cancellationToken = default)
    {
        var payload = new Dictionary<string, object>();
        if (types != null && types.Count > 0)
        {
            payload["types"] = types;
        }
        payload["max_wait_seconds"] = maxWaitSeconds;
        using var req = new HttpRequestMessage(HttpMethod.Post, _baseUrl + "/api/jobs/claim");
        AddAuth(req);
        req.Content = new StringContent(JsonSerializer.Serialize(payload), Encoding.UTF8, "application/json");
        using var resp = await _http.SendAsync(req, cancellationToken).ConfigureAwait(false);
        if (resp.StatusCode == System.Net.HttpStatusCode.NoContent)
        {
            return null;
        }
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
        return JsonSerializer.Deserialize<JobClaimResponse>(json, JsonOptions.Default)
               ?? throw new InvalidOperationException("Invalid claim response");
    }

    public async Task<TransferRequestResponse> RequestPayloadChannelAsync(
        string jobId,
        string? key,
        CancellationToken cancellationToken = default)
    {
        using var req = new HttpRequestMessage(HttpMethod.Post, _baseUrl + $"/api/jobs/{jobId}/payload");
        AddAuth(req);
        if (key != null)
        {
            req.Content = new StringContent(JsonSerializer.Serialize(new TransferRequest { Key = key }), Encoding.UTF8, "application/json");
        }
        using var resp = await _http.SendAsync(req, cancellationToken).ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
        return JsonSerializer.Deserialize<TransferRequestResponse>(json, JsonOptions.Default)
               ?? throw new InvalidOperationException("Invalid payload response");
    }

    public async Task<TransferRequestResponse> RequestResultChannelAsync(
        string jobId,
        string? key,
        CancellationToken cancellationToken = default)
    {
        using var req = new HttpRequestMessage(HttpMethod.Post, _baseUrl + $"/api/jobs/{jobId}/result");
        AddAuth(req);
        if (key != null)
        {
            req.Content = new StringContent(JsonSerializer.Serialize(new TransferRequest { Key = key }), Encoding.UTF8, "application/json");
        }
        using var resp = await _http.SendAsync(req, cancellationToken).ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
        return JsonSerializer.Deserialize<TransferRequestResponse>(json, JsonOptions.Default)
               ?? throw new InvalidOperationException("Invalid result response");
    }

    public async Task UpdateStatusAsync(
        string jobId,
        string state,
        Dictionary<string, object>? progress = null,
        JobError? error = null,
        CancellationToken cancellationToken = default)
    {
        var body = new Dictionary<string, object> { { "state", state } };
        if (progress != null)
        {
            body["progress"] = progress;
        }
        if (error != null)
        {
            body["error"] = error;
        }
        using var req = new HttpRequestMessage(HttpMethod.Post, _baseUrl + $"/api/jobs/{jobId}/status");
        AddAuth(req);
        req.Content = new StringContent(JsonSerializer.Serialize(body), Encoding.UTF8, "application/json");
        using var resp = await _http.SendAsync(req, cancellationToken).ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();
    }

    public async Task<byte[]> ReadPayloadAsync(string urlPathOrId, CancellationToken cancellationToken = default)
    {
        using var req = new HttpRequestMessage(HttpMethod.Get, ResolveUrl(urlPathOrId));
        AddAuth(req);
        using var resp = await _http.SendAsync(req, cancellationToken).ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();
        return await resp.Content.ReadAsByteArrayAsync(cancellationToken).ConfigureAwait(false);
    }

    public async Task WriteResultAsync(string urlPathOrId, byte[] data, string? contentType = null, CancellationToken cancellationToken = default)
    {
        using var req = new HttpRequestMessage(HttpMethod.Post, ResolveUrl(urlPathOrId));
        AddAuth(req);
        req.Content = new ByteArrayContent(data);
        if (!string.IsNullOrWhiteSpace(contentType))
        {
            req.Content.Headers.ContentType = MediaTypeHeaderValue.Parse(contentType);
        }
        using var resp = await _http.SendAsync(req, cancellationToken).ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();
    }

    public async Task<bool> RunOnceAsync(
        Func<JobClaimResponse, byte[], Task<(byte[] Result, string? ErrorMessage)>> handler,
        List<string>? types,
        int maxWaitSeconds,
        WorkerStatusHandler? onStatus = null,
        CancellationToken cancellationToken = default)
    {
        var job = await ClaimJobAsync(types, maxWaitSeconds, cancellationToken).ConfigureAwait(false);
        if (job == null)
        {
            return false;
        }
        var jobId = job.JobId;
        await UpdateStatusAsync(jobId, "claimed", cancellationToken: cancellationToken).ConfigureAwait(false);
        if (onStatus != null)
        {
            await onStatus("claimed", null).ConfigureAwait(false);
        }

        var payloadChannel = await RequestPayloadChannelAsync(jobId, null, cancellationToken).ConfigureAwait(false);
        if (onStatus != null)
        {
            await onStatus("awaiting_payload", payloadChannel).ConfigureAwait(false);
        }
        var payload = await ReadPayloadAsync(payloadChannel.ReaderUrl ?? payloadChannel.ChannelId, cancellationToken)
            .ConfigureAwait(false);

        await UpdateStatusAsync(jobId, "running", cancellationToken: cancellationToken).ConfigureAwait(false);
        if (onStatus != null)
        {
            await onStatus("running", null).ConfigureAwait(false);
        }

        var (result, errorMessage) = await handler(job, payload).ConfigureAwait(false);
        if (!string.IsNullOrWhiteSpace(errorMessage))
        {
            var error = new JobError { Code = "handler_error", Message = errorMessage };
            await UpdateStatusAsync(jobId, "failed", error: error, cancellationToken: cancellationToken).ConfigureAwait(false);
            if (onStatus != null)
            {
                await onStatus("failed", error).ConfigureAwait(false);
            }
            return true;
        }

        var resultChannel = await RequestResultChannelAsync(jobId, null, cancellationToken).ConfigureAwait(false);
        if (onStatus != null)
        {
            await onStatus("awaiting_result", resultChannel).ConfigureAwait(false);
        }
        await WriteResultAsync(resultChannel.WriterUrl ?? resultChannel.ChannelId, result, "application/octet-stream", cancellationToken)
            .ConfigureAwait(false);

        await UpdateStatusAsync(jobId, "completed", cancellationToken: cancellationToken).ConfigureAwait(false);
        if (onStatus != null)
        {
            await onStatus("completed", null).ConfigureAwait(false);
        }
        return true;
    }

    private void AddAuth(HttpRequestMessage req)
    {
        if (!string.IsNullOrWhiteSpace(_clientKey))
        {
            req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", _clientKey);
        }
    }

    private string ResolveUrl(string urlPathOrId)
    {
        if (urlPathOrId.StartsWith("http://", StringComparison.OrdinalIgnoreCase) ||
            urlPathOrId.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
        {
            return urlPathOrId;
        }
        if (!urlPathOrId.Contains("/", StringComparison.Ordinal))
        {
            return $"{_baseUrl}/api/transfer/{urlPathOrId}";
        }
        return $"{_baseUrl}{urlPathOrId}";
    }

    public void Dispose()
    {
        if (_ownsHttp)
        {
            _http.Dispose();
        }
    }
}

public sealed class JobCreateResponse
{
    [JsonPropertyName("job_id")]
    public string JobId { get; set; } = string.Empty;
    public string Status { get; set; } = string.Empty;
}

public sealed class JobClaimResponse
{
    [JsonPropertyName("job_id")]
    public string JobId { get; set; } = string.Empty;
    public string Type { get; set; } = string.Empty;
    public Dictionary<string, JsonElement>? Metadata { get; set; }
}

public sealed class TransferRequest
{
    public string? Key { get; set; }
}

public sealed class TransferRequestResponse
{
    public string? Key { get; set; }
    [JsonPropertyName("channel_id")]
    public string ChannelId { get; set; } = string.Empty;
    [JsonPropertyName("reader_url")]
    public string? ReaderUrl { get; set; }
    [JsonPropertyName("writer_url")]
    public string? WriterUrl { get; set; }
    [JsonPropertyName("expires_at")]
    public string ExpiresAt { get; set; } = string.Empty;
}

public sealed class JobError
{
    public string Code { get; set; } = string.Empty;
    public string Message { get; set; } = string.Empty;
}

public sealed class JobSession
{
    public string JobId { get; }
    public PayloadProvider PayloadProvider { get; }
    public ResultConsumer ResultConsumer { get; }

    public JobSession(string jobId, PayloadProvider payloadProvider, ResultConsumer resultConsumer)
    {
        JobId = jobId;
        PayloadProvider = payloadProvider;
        ResultConsumer = resultConsumer;
    }
}

public delegate Task<(byte[] Data, string? ContentType)?> PayloadProvider(string key, Dictionary<string, JsonElement> payload);
public delegate Task ResultConsumer(string key, byte[] data, Dictionary<string, JsonElement> payload);
public delegate Task StatusHandler(Dictionary<string, JsonElement> status);
public delegate Task WorkerStatusHandler(string state, object? info);
