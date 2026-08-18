"""Drive a TUI through a pty and render the resulting screen exactly."""
import os, pty, time, fcntl, termios, struct, select
import pyte

def _respond(fd, chunk):
    """Answer the terminal capability queries Bubble Tea sends on startup.

    Without replies the program blocks forever waiting for a terminal that
    never speaks back.
    """
    if b"\x1b]11;?" in chunk:                       # background colour query
        os.write(fd, b"\x1b]11;rgb:1e1e/1e1e/1e1e\x1b\\")
    if b"\x1b[6n" in chunk:                          # cursor position report
        os.write(fd, b"\x1b[1;1R")
    if b"\x1b[?2026$p" in chunk:                     # synchronised output
        os.write(fd, b"\x1b[?2026;2$y")

def drive(argv, keys=(), cols=120, rows=30, env=None, settle=0.6, quit_key=b"q"):
    pid, fd = pty.fork()
    if pid == 0:
        os.environ["TERM"] = "xterm-256color"
        os.environ["COLORTERM"] = "truecolor"
        os.environ["LINES"] = str(rows); os.environ["COLUMNS"] = str(cols)
        if env: os.environ.update(env)
        os.execvp(argv[0], argv); os._exit(127)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))

    buf = bytearray()
    def pump(t):
        end = time.time() + t
        while time.time() < end:
            r, _, _ = select.select([fd], [], [], 0.05)
            if not r: continue
            try: d = os.read(fd, 65536)
            except OSError: return False
            if not d: return False
            buf.extend(d); _respond(fd, d)
        return True

    pump(settle)
    for k in keys:
        os.write(fd, k.encode() if isinstance(k, str) else k)
        pump(settle)
    # Kill rather than quit: leaving the alt screen restores the previous
    # buffer, which would overwrite the frame we came to look at.
    import signal
    try: os.kill(pid, signal.SIGKILL)
    except ProcessLookupError: pass
    try: os.close(fd)
    except OSError: pass
    try: os.waitpid(pid, 0)
    except ChildProcessError: pass
    return bytes(buf)

def render(buf, cols=120, rows=30):
    screen = pyte.Screen(cols, rows)
    stream = pyte.Stream(screen)
    stream.feed(buf.decode("utf-8", "replace"))
    return "\n".join(l.rstrip() for l in screen.display)

def shot(argv, keys=(), cols=120, rows=30, env=None, settle=0.6):
    return render(drive(argv, keys, cols, rows, env, settle), cols, rows)
