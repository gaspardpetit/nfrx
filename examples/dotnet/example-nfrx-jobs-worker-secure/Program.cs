using System;
using System.Collections.Generic;
using System.IO;
using System.Text.Json;
using System.Threading.Tasks;
using NfrxSdk;

class Program
{
    public static async Task<int> Main()
    {
        var baseUrl = Environment.GetEnvironmentVariable("NFRX_BASE_URL") ?? "http://localhost:8080";
        var clientKey = Environment.GetEnvironmentVariable("NFRX_CLIENT_KEY");
        var debugResultFile = Environment.GetEnvironmentVariable("NFRX_DEBUG_RESULT_FILE");

        using var identity = SecureTransfer.GenerateSelfSignedIdentity("nfrx secure worker");
        Console.WriteLine("generated worker certificate (PEM):");
        Console.WriteLine(identity.CertificatePem.Trim());

        using var worker = new NfrxJobsWorker(baseUrl, clientKey);
        while (true)
        {
            var job = await worker.ClaimJobAsync(new List<string> { "asr.transcribe" }, 30).ConfigureAwait(false);
            if (job == null)
            {
                Console.WriteLine("no job available");
                continue;
            }

            Console.WriteLine($"claimed secure job: {job.JobId}");
            try
            {
                var secure = GetSecureMetadata(job);
                await worker.UpdateStatusAsync(job.JobId, "claimed").ConfigureAwait(false);

                var payloadChannel = await worker.RequestPayloadChannelAsync(
                    job.JobId,
                    null,
                    new Dictionary<string, object>
                    {
                        ["encryption_scheme"] = SecureTransfer.Scheme,
                        ["envelope"] = SecureTransfer.Envelope,
                        ["recipient_cert_pem"] = identity.CertificatePem,
                    }).ConfigureAwait(false);
                var payloadCiphertext = await worker.ReadPayloadAsync(payloadChannel.ReaderUrl ?? payloadChannel.ChannelId).ConfigureAwait(false);
                var payloadPlaintext = SecureTransfer.DecryptCms(payloadCiphertext, identity.Certificate);
                var payloadEnvelope = SecureTransfer.ParseHeaderBodyEnvelope(payloadPlaintext);
                Console.WriteLine("payload headers: " + JsonSerializer.Serialize(payloadEnvelope.Headers));

                await worker.UpdateStatusAsync(job.JobId, "running").ConfigureAwait(false);

                var resultEnvelope = SecureTransfer.BuildHeaderBodyEnvelope(
                    payloadEnvelope.Headers["Content-Type"],
                    payloadEnvelope.Body,
                    "worker-result");
                var resultCiphertext = SecureTransfer.EncryptCms(resultEnvelope, secure.ResultRecipientCertPem);
                var resultChannel = await worker.RequestResultChannelAsync(
                    job.JobId,
                    null,
                    new Dictionary<string, object>
                    {
                        ["encryption_scheme"] = SecureTransfer.Scheme,
                        ["envelope"] = SecureTransfer.Envelope,
                        ["recipient"] = "job-metadata-result-cert",
                    }).ConfigureAwait(false);
                await worker.WriteResultAsync(
                    resultChannel.WriterUrl ?? resultChannel.ChannelId,
                    resultCiphertext,
                    SecureTransfer.CmsContentType).ConfigureAwait(false);
                if (!string.IsNullOrWhiteSpace(debugResultFile))
                {
                    File.WriteAllBytes(debugResultFile, payloadEnvelope.Body);
                }
                await worker.UpdateStatusAsync(job.JobId, "completed").ConfigureAwait(false);
                Console.WriteLine($"completed secure job: {job.JobId}");
                return 0;
            }
            catch (Exception exc)
            {
                await worker.UpdateStatusAsync(
                    job.JobId,
                    "failed",
                    error: new JobError { Code = "secure_transfer_error", Message = exc.Message }).ConfigureAwait(false);
                Console.WriteLine("error: " + exc.Message);
                return 1;
            }
        }
    }

    private static SecureMetadata GetSecureMetadata(JobClaimResponse job)
    {
        if (job.Metadata == null || !job.Metadata.TryGetValue("secure_transfer", out var secureElement) || secureElement.ValueKind != JsonValueKind.Object)
        {
            throw new InvalidOperationException("Job metadata missing secure_transfer");
        }
        if (!secureElement.TryGetProperty("supported_schemes", out var schemes) || schemes.ValueKind != JsonValueKind.Array)
        {
            throw new InvalidOperationException("Job metadata missing supported_schemes");
        }
        var supportsCms = false;
        foreach (var item in schemes.EnumerateArray())
        {
            if (item.ValueKind == JsonValueKind.String && item.GetString() == SecureTransfer.Scheme)
            {
                supportsCms = true;
                break;
            }
        }
        if (!supportsCms)
        {
            throw new InvalidOperationException($"Client does not advertise {SecureTransfer.Scheme}");
        }
        var envelope = RequireNonEmptyString(secureElement, "envelope");
        if (!string.Equals(envelope, SecureTransfer.Envelope, StringComparison.Ordinal))
        {
            throw new InvalidOperationException($"Unsupported envelope: {envelope}");
        }
        return new SecureMetadata(RequireNonEmptyString(secureElement, "result_recipient_cert_pem"));
    }

    private static string RequireNonEmptyString(JsonElement obj, string propertyName)
    {
        if (!obj.TryGetProperty(propertyName, out var value) || value.ValueKind != JsonValueKind.String)
        {
            throw new InvalidOperationException($"Missing string property {propertyName}");
        }
        return value.GetString() ?? throw new InvalidOperationException($"{propertyName} is null");
    }

    private sealed record SecureMetadata(string ResultRecipientCertPem);
}
