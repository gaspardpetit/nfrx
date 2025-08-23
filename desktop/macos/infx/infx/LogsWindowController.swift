import Cocoa

class LogsWindowController: NSWindowController {
    private let outTextView = NSTextView()
    private let errTextView = NSTextView()
    private var outHandle: FileHandle?
    private var errHandle: FileHandle?
    private var outSource: DispatchSourceFileSystemObject?
    private var errSource: DispatchSourceFileSystemObject?

    init() {
        let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 600, height: 400),
                              styleMask: [.titled, .closable, .resizable],
                              backing: .buffered, defer: false)
        window.title = "Worker Logs"
        let split = NSSplitView(frame: window.contentView!.bounds)
        split.isVertical = false
        split.autoresizingMask = [.width, .height]
        let outScroll = NSScrollView()
        outScroll.hasVerticalScroller = true
        outScroll.documentView = outTextView
        outTextView.isEditable = false
        let errScroll = NSScrollView()
        errScroll.hasVerticalScroller = true
        errScroll.documentView = errTextView
        errTextView.isEditable = false
        split.addArrangedSubview(outScroll)
        split.addArrangedSubview(errScroll)
        window.contentView?.addSubview(split)
        super.init(window: window)
        tailLogs()
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    private func tailLogs() {
        tail(file: ConfigManager.shared.logsDirURL.appendingPathComponent("worker.out"),
             textView: outTextView,
             handle: &outHandle,
             source: &outSource)
        tail(file: ConfigManager.shared.logsDirURL.appendingPathComponent("worker.err"),
             textView: errTextView,
             handle: &errHandle,
             source: &errSource)
    }

    private func tail(file: URL, textView: NSTextView, handle: inout FileHandle?, source: inout DispatchSourceFileSystemObject?) {
        guard FileManager.default.fileExists(atPath: file.path),
              let fh = try? FileHandle(forReadingFrom: file) else { return }
        handle = fh
        fh.seekToEndOfFile()
        source = DispatchSource.makeFileSystemObjectSource(fileDescriptor: fh.fileDescriptor,
                                                            eventMask: .extend,
                                                            queue: DispatchQueue.global())
        source?.setEventHandler {
            let data = fh.readDataToEndOfFile()
            if let str = String(data: data, encoding: .utf8) {
                DispatchQueue.main.async {
                    textView.string += str
                    textView.scrollToEndOfDocument(nil)
                }
            }
        }
        source?.resume()
    }

    deinit {
        outSource?.cancel()
        errSource?.cancel()
        try? outHandle?.close()
        try? errHandle?.close()
    }
}
