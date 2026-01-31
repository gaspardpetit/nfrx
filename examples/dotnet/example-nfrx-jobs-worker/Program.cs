// Minimal jobs worker example using nfrx-sdk.
//
// Environment:
//   NFRX_BASE_URL  (default http://localhost:8080)
//   NFRX_CLIENT_KEY (optional)
//
// Run:
//   dotnet run --project examples/dotnet/example-nfrx-jobs-worker

using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using NfrxSdk;

class Program
{
    public static async Task<int> Main()
    {
        var baseUrl = Environment.GetEnvironmentVariable("NFRX_BASE_URL") ?? "http://localhost:8080";
        var clientKey = Environment.GetEnvironmentVariable("NFRX_CLIENT_KEY");

        using var worker = new NfrxJobsWorker(baseUrl, clientKey);
        while (true)
        {
            var handled = await worker.RunOnceAsync(
                handler: async (job, payload) =>
                {
                    _ = job;
                    return (payload, (string?)null);
                },
                types: new List<string> { "asr.transcribe" },
                maxWaitSeconds: 30,
                onStatus: async (state, info) =>
                {
                    Console.WriteLine($"status: {state}");
                    await Task.CompletedTask;
                }
            );

            if (handled)
            {
                return 0;
            }
        }
    }
}
