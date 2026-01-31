// Minimal transfer example using nfrx-sdk.
//
// Environment:
//   NFRX_BASE_URL (default http://localhost:8080)
//   NFRX_API_KEY  (optional)
//
// Run:
//   dotnet run --project examples/dotnet/example-nfrx-transfer

using System;
using System.Text;
using System.Threading.Tasks;
using NfrxSdk;

class Program
{
    public static async Task<int> Main()
    {
        var baseUrl = Environment.GetEnvironmentVariable("NFRX_BASE_URL") ?? "http://localhost:8080";
        var apiKey = Environment.GetEnvironmentVariable("NFRX_API_KEY");

        using var client = new NfrxTransferClient(baseUrl, apiKey);
        var channel = await client.CreateChannelAsync();
        Console.WriteLine($"channel_id: {channel.ChannelId}");

        var payload = Encoding.UTF8.GetBytes("hello world");
        await client.UploadAsync(channel.ChannelId, payload, "application/octet-stream");
        var echo = await client.DownloadAsync(channel.ChannelId);
        Console.WriteLine($"echo: {Encoding.UTF8.GetString(echo)}");

        return 0;
    }
}
