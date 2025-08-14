using System;
using System.Windows.Forms;

namespace TrayApp;

public class PreferencesForm : Form
{
    private readonly TextBox _serverUrl;
    private readonly TextBox _workerKey;
    private readonly TextBox _ollamaUrl;
    private readonly NumericUpDown _maxConcurrency;
    private readonly NumericUpDown _statusPort;

    public WorkerConfig Config { get; private set; }

    public PreferencesForm(WorkerConfig config)
    {
        Text = "Preferences";
        FormBorderStyle = FormBorderStyle.FixedDialog;
        StartPosition = FormStartPosition.CenterParent;
        MinimizeBox = false;
        MaximizeBox = false;
        Config = config;

        _serverUrl = new TextBox { Dock = DockStyle.Fill, Text = config.ServerUrl };
        _workerKey = new TextBox { Dock = DockStyle.Fill, Text = config.WorkerKey };
        _ollamaUrl = new TextBox { Dock = DockStyle.Fill, Text = config.OllamaBaseUrl };
        _maxConcurrency = new NumericUpDown { Dock = DockStyle.Fill, Minimum = 1, Maximum = 128, Value = config.MaxConcurrency };
        _statusPort = new NumericUpDown { Dock = DockStyle.Fill, Minimum = 1, Maximum = 65535, Value = config.StatusPort };

        var table = new TableLayoutPanel { Dock = DockStyle.Top, ColumnCount = 2, RowCount = 5, AutoSize = true };
        table.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        table.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));

        table.Controls.Add(new Label { Text = "Server URL", Anchor = AnchorStyles.Left, AutoSize = true }, 0, 0);
        table.Controls.Add(_serverUrl, 1, 0);
        table.Controls.Add(new Label { Text = "Worker Key", Anchor = AnchorStyles.Left, AutoSize = true }, 0, 1);
        table.Controls.Add(_workerKey, 1, 1);
        table.Controls.Add(new Label { Text = "Ollama Base URL", Anchor = AnchorStyles.Left, AutoSize = true }, 0, 2);
        table.Controls.Add(_ollamaUrl, 1, 2);
        table.Controls.Add(new Label { Text = "Max Concurrency", Anchor = AnchorStyles.Left, AutoSize = true }, 0, 3);
        table.Controls.Add(_maxConcurrency, 1, 3);
        table.Controls.Add(new Label { Text = "Status Port", Anchor = AnchorStyles.Left, AutoSize = true }, 0, 4);
        table.Controls.Add(_statusPort, 1, 4);

        var buttons = new FlowLayoutPanel { Dock = DockStyle.Bottom, FlowDirection = FlowDirection.RightToLeft, AutoSize = true };
        var saveButton = new Button { Text = "Save", DialogResult = DialogResult.OK };
        var cancelButton = new Button { Text = "Cancel", DialogResult = DialogResult.Cancel };
        buttons.Controls.Add(saveButton);
        buttons.Controls.Add(cancelButton);
        AcceptButton = saveButton;
        CancelButton = cancelButton;

        Controls.Add(table);
        Controls.Add(buttons);

        saveButton.Click += (_, _) => UpdateConfig();
    }

    private void UpdateConfig()
    {
        Config.ServerUrl = _serverUrl.Text.Trim();
        Config.WorkerKey = _workerKey.Text.Trim();
        Config.OllamaBaseUrl = _ollamaUrl.Text.Trim();
        Config.MaxConcurrency = (int)_maxConcurrency.Value;
        Config.StatusPort = (int)_statusPort.Value;
    }
}

