using System;
using System.Collections.Generic;
using System.IO;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
using NfrxSdk;

class Program
{
    public static async Task<int> Main()
    {
        var baseUrl = Environment.GetEnvironmentVariable("NFRX_BASE_URL") ?? "http://localhost:8080";
        var apiKey = Environment.GetEnvironmentVariable("NFRX_API_KEY");
        var payloadFile = Environment.GetEnvironmentVariable("NFRX_PAYLOAD_FILE");
        var resultFile = Environment.GetEnvironmentVariable("NFRX_RESULT_FILE");

        using var identity = SecureTransfer.GenerateSelfSignedIdentity("nfrx secure client");
        Console.WriteLine("generated client certificate (PEM):");
        Console.WriteLine(identity.CertificatePem.Trim());

        using var http = new HttpClient();
        var jobs = new NfrxJobsClient(baseUrl, apiKey, http);
        var transfer = new NfrxTransferClient(baseUrl, apiKey, http);

        var metadata = new Dictionary<string, object>
        {
            ["language"] = "en",
            ["secure_transfer"] = new Dictionary<string, object>
            {
                ["supported_schemes"] = new[] { SecureTransfer.Scheme },
                ["result_recipient_cert_pem"] = identity.CertificatePem,
                ["envelope"] = SecureTransfer.Envelope,
            },
        };

        var created = await jobs.CreateJobAsync("asr.transcribe", metadata);
        Console.WriteLine($"created secure job: {created.JobId}");

        using var cts = new CancellationTokenSource(TimeSpan.FromSeconds(30));
        await foreach (var (type, data) in jobs.StreamEventsAsync(created.JobId, cts.Token))
        {
            if (type == "status")
            {
                Console.WriteLine("status: " + data.GetValueOrDefault("status"));
                if (IsTerminal(data))
                {
                    return 0;
                }
                continue;
            }

            if (type == "payload")
            {
                var recipientPem = ValidatePayloadProperties(data);
                var envelope = SecureTransfer.BuildHeaderBodyEnvelope(
                    "application/octet-stream",
                    ReadPayload(payloadFile),
                    "client-payload");
                var ciphertext = SecureTransfer.EncryptCms(envelope, recipientPem);
                var channel = GetChannel(data);
                await transfer.UploadAsync(channel, ciphertext, SecureTransfer.CmsContentType, cts.Token);
                Console.WriteLine($"uploaded encrypted payload ({ciphertext.Length} bytes)");
                continue;
            }

            if (type == "result")
            {
                ValidateResultProperties(data);
                var channel = GetChannel(data);
                var ciphertext = await transfer.DownloadAsync(channel, cts.Token);
                var plaintext = SecureTransfer.DecryptCms(ciphertext, identity.Certificate);
                var envelope = SecureTransfer.ParseHeaderBodyEnvelope(plaintext);
                Console.WriteLine("result headers: " + JsonSerializer.Serialize(envelope.Headers));
                WriteResult(resultFile, envelope.Body);
                Console.WriteLine($"received encrypted result ({ciphertext.Length} bytes)");
            }
        }

        return 0;
    }

    private static byte[] ReadPayload(string? path)
    {
        return string.IsNullOrWhiteSpace(path) ? "hello world"u8.ToArray() : File.ReadAllBytes(path);
    }

    private static void WriteResult(string? path, byte[] data)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            Console.WriteLine("result body: " + System.Text.Encoding.UTF8.GetString(data));
            return;
        }
        File.WriteAllBytes(path, data);
    }

    private static string GetChannel(Dictionary<string, JsonElement> payload)
    {
        if (payload.TryGetValue("channel_id", out var channelId) && channelId.ValueKind == JsonValueKind.String)
        {
            return channelId.GetString() ?? throw new InvalidOperationException("channel_id is null");
        }
        if (payload.TryGetValue("url", out var url) && url.ValueKind == JsonValueKind.String)
        {
            return url.GetString() ?? throw new InvalidOperationException("url is null");
        }
        throw new InvalidOperationException("Missing transfer channel info");
    }

    private static string ValidatePayloadProperties(Dictionary<string, JsonElement> payload)
    {
        var properties = GetProperties(payload);
        RequireString(properties, "encryption_scheme", SecureTransfer.Scheme);
        RequireString(properties, "envelope", SecureTransfer.Envelope);
        return RequireNonEmptyString(properties, "recipient_cert_pem");
    }

    private static void ValidateResultProperties(Dictionary<string, JsonElement> payload)
    {
        var properties = GetProperties(payload);
        RequireString(properties, "encryption_scheme", SecureTransfer.Scheme);
        RequireString(properties, "envelope", SecureTransfer.Envelope);
        RequireString(properties, "recipient", "job-metadata-result-cert");
    }

    private static JsonElement GetProperties(Dictionary<string, JsonElement> payload)
    {
        if (!payload.TryGetValue("properties", out var properties) || properties.ValueKind != JsonValueKind.Object)
        {
            throw new InvalidOperationException("Event missing properties object");
        }
        return properties;
    }

    private static void RequireString(JsonElement obj, string propertyName, string expected)
    {
        var actual = RequireNonEmptyString(obj, propertyName);
        if (!string.Equals(actual, expected, StringComparison.Ordinal))
        {
            throw new InvalidOperationException($"Unexpected {propertyName}: {actual}");
        }
    }

    private static string RequireNonEmptyString(JsonElement obj, string propertyName)
    {
        if (!obj.TryGetProperty(propertyName, out var value) || value.ValueKind != JsonValueKind.String)
        {
            throw new InvalidOperationException($"Missing string property {propertyName}");
        }
        return value.GetString() ?? throw new InvalidOperationException($"{propertyName} is null");
    }

    private static bool IsTerminal(Dictionary<string, JsonElement> payload)
    {
        if (!payload.TryGetValue("status", out var status) || status.ValueKind != JsonValueKind.String)
        {
            return false;
        }
        var value = status.GetString();
        return value == "completed" || value == "failed" || value == "canceled";
    }
}
