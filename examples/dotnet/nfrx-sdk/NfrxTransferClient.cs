using System;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading;
using System.Threading.Tasks;

namespace NfrxSdk;

public sealed class NfrxTransferClient : IDisposable
{
    private readonly HttpClient _http;
    private readonly bool _ownsClient;
    private readonly string _baseUrl;
    private readonly string? _bearerToken;

    public NfrxTransferClient(string baseUrl, string? bearerToken = null, HttpClient? httpClient = null)
    {
        _baseUrl = baseUrl.TrimEnd('/');
        _bearerToken = bearerToken;
        if (httpClient == null)
        {
            _http = new HttpClient();
            _ownsClient = true;
        }
        else
        {
            _http = httpClient;
            _ownsClient = false;
        }
    }

    public async Task<TransferCreateResponse> CreateChannelAsync(CancellationToken cancellationToken = default)
    {
        using var req = new HttpRequestMessage(HttpMethod.Post, ResolveUrl("/api/transfer"));
        AddAuth(req);
        using var resp = await _http.SendAsync(req, cancellationToken).ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
        return JsonSerializer.Deserialize<TransferCreateResponse>(json, JsonOptions.Default)
               ?? throw new InvalidOperationException("Invalid create channel response");
    }

    public async Task<byte[]> DownloadAsync(string urlPathOrId, CancellationToken cancellationToken = default)
    {
        using var req = new HttpRequestMessage(HttpMethod.Get, ResolveUrl(urlPathOrId));
        AddAuth(req);
        using var resp = await _http.SendAsync(req, cancellationToken).ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();
        return await resp.Content.ReadAsByteArrayAsync(cancellationToken).ConfigureAwait(false);
    }

    public async Task UploadAsync(
        string urlPathOrId,
        byte[] data,
        string? contentType = null,
        CancellationToken cancellationToken = default)
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

    private void AddAuth(HttpRequestMessage req)
    {
        if (!string.IsNullOrWhiteSpace(_bearerToken))
        {
            req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", _bearerToken);
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
        if (_ownsClient)
        {
            _http.Dispose();
        }
    }
}

public sealed class TransferCreateResponse
{
    [JsonPropertyName("channel_id")]
    public string ChannelId { get; set; } = string.Empty;
    [JsonPropertyName("expires_at")]
    public string ExpiresAt { get; set; } = string.Empty;
}

internal static class JsonOptions
{
    public static readonly JsonSerializerOptions Default = new()
    {
        PropertyNameCaseInsensitive = true
    };
}
