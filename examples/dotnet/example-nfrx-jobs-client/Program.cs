// Minimal jobs client example using nfrx-sdk.
//
// Environment:
//   NFRX_BASE_URL (default http://localhost:8080)
//   NFRX_API_KEY  (optional)
//
// Run:
//   dotnet run --project examples/dotnet/example-nfrx-jobs-client

using System;
using System.Collections.Generic;
using System.Text;
using System.Threading.Tasks;
using NfrxSdk;

class Program
{
    public static async Task<int> Main()
    {
        var baseUrl = Environment.GetEnvironmentVariable("NFRX_BASE_URL") ?? "http://localhost:8080";
        var apiKey = Environment.GetEnvironmentVariable("NFRX_API_KEY");

        using var runner = new NfrxJobsRunner(baseUrl, apiKey, timeoutSeconds: 30);
        var session = await runner.CreateJobSessionAsync(
            jobType: "asr.transcribe",
            metadata: new Dictionary<string, object> { { "language", "en" } },
            payloadProvider: async (key, payload) =>
            {
                _ = key;
                _ = payload;
                return (Encoding.UTF8.GetBytes("hello world"), "application/octet-stream");
            },
            resultConsumer: async (key, data, payload) =>
            {
                _ = key;
                _ = payload;
                Console.WriteLine("result: " + Encoding.UTF8.GetString(data));
                await Task.CompletedTask;
            }
        );

        await runner.RunSessionAsync(session, status =>
        {
            Console.WriteLine("status: " + status.GetValueOrDefault("status"));
            return Task.CompletedTask;
        }, timeoutSeconds: 30);

        return 0;
    }
}
