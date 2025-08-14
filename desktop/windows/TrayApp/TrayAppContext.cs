using System;
using System.Drawing;
using System.Windows.Forms;

namespace TrayApp;

public class TrayAppContext : ApplicationContext
{
    private readonly NotifyIcon _notifyIcon;
    private readonly ToolStripMenuItem _statusItem;
    private readonly ToolStripMenuItem _startStopItem;
    private readonly ToolStripMenuItem _startWithWindowsItem;

    public TrayAppContext()
    {
        _statusItem = new ToolStripMenuItem("Status: Unknown")
        {
            Enabled = false
        };

        _startStopItem = new ToolStripMenuItem("Start Worker", null, OnStartStopClicked);

        _startWithWindowsItem = new ToolStripMenuItem("Start with Windows")
        {
            CheckOnClick = true
        };
        _startWithWindowsItem.Click += OnStartWithWindowsClicked;

        var contextMenu = new ContextMenuStrip();
        contextMenu.Items.AddRange(new ToolStripItem[]
        {
            _statusItem,
            new ToolStripSeparator(),
            _startStopItem,
            new ToolStripMenuItem("Preferences...", null, OnPreferencesClicked),
            new ToolStripMenuItem("Logs...", null, OnLogsClicked),
            _startWithWindowsItem,
            new ToolStripMenuItem("Check for Updates", null, OnCheckForUpdatesClicked),
            new ToolStripMenuItem("Exit", null, OnExitClicked)
        });

        _notifyIcon = new NotifyIcon
        {
            Icon = SystemIcons.Application,
            ContextMenuStrip = contextMenu,
            Visible = true,
            Text = "llamapool"
        };
    }

    private void OnStartStopClicked(object? sender, EventArgs e)
    {
        // TODO: Start or stop the worker service
        MessageBox.Show("Start/Stop Worker clicked");
    }

    private void OnPreferencesClicked(object? sender, EventArgs e)
    {
        // TODO: Open preferences dialog
        MessageBox.Show("Preferences clicked");
    }

    private void OnLogsClicked(object? sender, EventArgs e)
    {
        // TODO: Open logs viewer
        MessageBox.Show("Logs clicked");
    }

    private void OnStartWithWindowsClicked(object? sender, EventArgs e)
    {
        // TODO: Toggle start with Windows
        MessageBox.Show("Start with Windows toggled");
    }

    private void OnCheckForUpdatesClicked(object? sender, EventArgs e)
    {
        // TODO: Check for application updates
        MessageBox.Show("Check for Updates clicked");
    }

    private void OnExitClicked(object? sender, EventArgs e)
    {
        _notifyIcon.Visible = false;
        Application.Exit();
    }
}
