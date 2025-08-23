using Microsoft.Extensions.Hosting.WindowsServices;
using WindowsService;

var builder = Host.CreateApplicationBuilder(args);
builder.Services.AddWindowsService(options =>
{
    options.ServiceName = "nfrx";
});
builder.Services.AddHostedService<Worker>();

var host = builder.Build();
host.Run();
