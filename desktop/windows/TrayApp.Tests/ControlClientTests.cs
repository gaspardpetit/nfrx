using System.Linq;
using System.Net;
using System.Net.Http;
using System.Threading;
using System.Threading.Tasks;
using TrayApp;
using Xunit;

public class ControlClientTests
{
    private class TestHandler : HttpMessageHandler
    {
        public HttpRequestMessage? LastRequest { get; private set; }
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
        {
            LastRequest = request;
            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK));
        }
    }

    [Fact]
    public async Task SendsTokenHeader()
    {
        var handler = new TestHandler();
        var client = new HttpClient(handler);
        var cc = new ControlClient(1234, client, token: "secret");

        await cc.SendCommandAsync("drain");

        Assert.NotNull(handler.LastRequest);
        Assert.Equal("secret", handler.LastRequest!.Headers.GetValues("X-Auth-Token").Single());
        Assert.Equal(HttpMethod.Post, handler.LastRequest.Method);
        Assert.Equal("http://127.0.0.1:1234/control/drain", handler.LastRequest.RequestUri!.ToString());
    }
}
