using System;
using System.IO;
using System.Net;
using System.Net.Http;
using System.Threading.Tasks;

namespace TrayApp;

public class ControlClient
{
    private readonly HttpClient _httpClient;
    private readonly string _baseUrl;
    private readonly string? _token;

    public ControlClient(int port = 4555, HttpClient? httpClient = null, string? token = null)
    {
        _httpClient = httpClient ?? new HttpClient();
        _baseUrl = $"http://127.0.0.1:{port}/control";
        if (!string.IsNullOrEmpty(token))
        {
            _token = token;
        }
        else
        {
            try
            {
                if (File.Exists(Paths.TokenPath))
                {
                    _token = File.ReadAllText(Paths.TokenPath).Trim();
                }
            }
            catch
            {
                // ignore
            }
        }
    }

    public async Task SendCommandAsync(string command)
    {
        using var req = CreateRequest(command, HttpMethod.Post);
        using var res = await _httpClient.SendAsync(req);
        res.EnsureSuccessStatusCode();
    }

    public async Task<bool> ProbeAsync()
    {
        try
        {
            using var req = CreateRequest("drain", HttpMethod.Get);
            using var res = await _httpClient.SendAsync(req);
            return res.StatusCode != HttpStatusCode.NotFound;
        }
        catch
        {
            return false;
        }
    }

    private HttpRequestMessage CreateRequest(string command, HttpMethod method)
    {
        var req = new HttpRequestMessage(method, $"{_baseUrl}/{command}");
        if (!string.IsNullOrEmpty(_token))
        {
            req.Headers.Add("X-Auth-Token", _token);
        }
        return req;
    }
}
