
using Timer = System.Windows.Forms.Timer;

namespace TrayApp;

/// <summary>
/// Simple read-only log viewer that tails the worker log file.
/// </summary>
public class LogsForm : Form
{
    private readonly string _logPath;
    private readonly TextBox _textBox;
    private long _lastLength;
    private readonly Timer _timer;

    public LogsForm(string logPath)
    {
        _logPath = logPath;
        Text = "Worker Logs";
        Width = 800;
        Height = 600;

        _textBox = new TextBox
        {
            Multiline = true,
            ReadOnly = true,
            ScrollBars = ScrollBars.Vertical,
            Dock = DockStyle.Fill,
            Font = new Font(FontFamily.GenericMonospace, 9f)
        };
        Controls.Add(_textBox);

        Load += async (_, _) => await LoadInitialAsync();

        _timer = new Timer { Interval = 1000 };
        _timer.Tick += async (_, _) => await AppendUpdatesAsync();
        _timer.Start();
    }

    protected override void OnFormClosed(FormClosedEventArgs e)
    {
        _timer.Stop();
        base.OnFormClosed(e);
    }

    private async Task LoadInitialAsync()
    {
        try
        {
            Directory.CreateDirectory(Path.GetDirectoryName(_logPath)!);
            using var fs = new FileStream(_logPath, FileMode.OpenOrCreate, FileAccess.Read, FileShare.ReadWrite);
            _lastLength = fs.Length;
            using var reader = new StreamReader(fs);
            _textBox.Text = await reader.ReadToEndAsync();
            _textBox.SelectionStart = _textBox.Text.Length;
            _textBox.ScrollToCaret();
        }
        catch (Exception ex)
        {
            _textBox.Text = $"Failed to read log file: {ex.Message}";
        }
    }

    private async Task AppendUpdatesAsync()
    {
        try
        {
            using var fs = new FileStream(_logPath, FileMode.OpenOrCreate, FileAccess.Read, FileShare.ReadWrite);
            if (fs.Length < _lastLength)
            {
                // Log rotated, reload from start
                fs.Seek(0, SeekOrigin.Begin);
                _lastLength = 0;
                _textBox.Clear();
            }
            else
            {
                fs.Seek(_lastLength, SeekOrigin.Begin);
            }

            using var reader = new StreamReader(fs);
            var text = await reader.ReadToEndAsync();
            if (text.Length > 0)
            {
                _textBox.AppendText(text);
                _lastLength = fs.Length;
                _textBox.SelectionStart = _textBox.Text.Length;
                _textBox.ScrollToCaret();
            }
        }
        catch
        {
            // Ignore read errors
        }
    }
}

