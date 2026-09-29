// Gives Go's os package a filesystem in the browser. Go's wasm runtime does
// file I/O through a Node-style globalThis.fs, which wasm_exec.js stubs out
// in browsers. This implements the calls Go makes on top of an emscripten
// module's FS (raylib's), so Go code and raylib see the same files: the
// assets bundled with --preload-file, plus anything either side writes.
"use strict";

function installGoFS(FS) {
	// Emscripten's errno values (WASI numbering) to the codes Go maps back to
	// syscall.Errno.
	const codes = {
		2: "EACCES", 8: "EBADF", 20: "EEXIST", 28: "EINVAL", 29: "EIO", 31: "EISDIR",
		32: "ELOOP", 37: "ENAMETOOLONG", 44: "ENOENT", 48: "ENOMEM", 51: "ENOSPC",
		52: "ENOSYS", 54: "ENOTDIR", 55: "ENOTEMPTY", 63: "EPERM", 69: "EROFS", 75: "EXDEV",
	};
	function goError(err) {
		if (!(err instanceof FS.ErrnoError)) throw err;
		const e = new Error(`${codes[err.errno] ?? "EIO"} (emscripten errno ${err.errno})`);
		e.code = codes[err.errno] ?? "EIO";
		return e;
	}
	// run calls f and reports its result the Node way: callback(err, value).
	function run(callback, f) {
		let value;
		try {
			value = f();
		} catch (err) {
			callback(goError(err));
			return;
		}
		callback(null, value);
	}
	const ms = (t) => (t instanceof Date ? t.getTime() : Number(t));
	function stats(s) {
		return {
			dev: s.dev, ino: s.ino, mode: s.mode, nlink: s.nlink, uid: s.uid, gid: s.gid,
			rdev: s.rdev, size: s.size, blksize: s.blksize, blocks: s.blocks,
			atimeMs: ms(s.atime), mtimeMs: ms(s.mtime), ctimeMs: ms(s.ctime),
			isDirectory: () => FS.isDir(s.mode),
		};
	}
	const stream = (fd) => (FS.getStreamChecked ? FS.getStreamChecked(fd) : FS.getStream(fd));

	// Standard output and error go to the console a line at a time, as in
	// wasm_exec.js's own stub (the Go runtime writes panics with writeSync).
	const decoder = new TextDecoder("utf-8");
	const pending = { 1: "", 2: "" };
	function writeConsole(fd, buf) {
		pending[fd] += decoder.decode(buf);
		const nl = pending[fd].lastIndexOf("\n");
		if (nl !== -1) {
			(fd === 2 ? console.warn : console.log)(pending[fd].substring(0, nl));
			pending[fd] = pending[fd].substring(nl + 1);
		}
		return buf.length;
	}

	globalThis.fs = {
		// Emscripten's flags follow Linux, which Go reads from here.
		constants: {
			O_WRONLY: 1, O_RDWR: 2, O_CREAT: 0o100, O_EXCL: 0o200, O_TRUNC: 0o1000,
			O_APPEND: 0o2000, O_DIRECTORY: 0o200000,
		},
		writeSync(fd, buf) {
			if (fd === 1 || fd === 2) return writeConsole(fd, buf);
			return FS.write(stream(fd), buf, 0, buf.length);
		},
		write(fd, buf, offset, length, position, callback) {
			if (fd === 1 || fd === 2) {
				callback(null, writeConsole(fd, buf.subarray(offset, offset + length)));
				return;
			}
			run(callback, () => FS.write(stream(fd), buf, offset, length, position ?? undefined));
		},
		read(fd, buf, offset, length, position, callback) {
			run(callback, () => FS.read(stream(fd), buf, offset, length, position ?? undefined));
		},
		open(path, flags, mode, callback) { run(callback, () => FS.open(path, flags, mode).fd); },
		close(fd, callback) { run(callback, () => FS.close(stream(fd))); },
		fsync(fd, callback) { run(callback, () => {}); },
		stat(path, callback) { run(callback, () => stats(FS.stat(path))); },
		lstat(path, callback) { run(callback, () => stats(FS.lstat(path))); },
		fstat(fd, callback) {
			run(callback, () => stats(FS.fstat ? FS.fstat(fd) : FS.stat(stream(fd).path)));
		},
		readdir(path, callback) {
			run(callback, () => FS.readdir(path).filter((n) => n !== "." && n !== ".."));
		},
		mkdir(path, perm, callback) { run(callback, () => FS.mkdir(path, perm)); },
		rmdir(path, callback) { run(callback, () => FS.rmdir(path)); },
		unlink(path, callback) { run(callback, () => FS.unlink(path)); },
		rename(from, to, callback) { run(callback, () => FS.rename(from, to)); },
		truncate(path, length, callback) { run(callback, () => FS.truncate(path, length)); },
		ftruncate(fd, length, callback) { run(callback, () => FS.ftruncate(fd, length)); },
		chmod(path, mode, callback) { run(callback, () => FS.chmod(path, mode)); },
		fchmod(fd, mode, callback) { run(callback, () => FS.fchmod(fd, mode)); },
		utimes(path, atime, mtime, callback) {
			run(callback, () => FS.utime(path, atime * 1000, mtime * 1000));
		},
		readlink(path, callback) { run(callback, () => FS.readlink(path)); },
		symlink(target, path, callback) { run(callback, () => FS.symlink(target, path)); },
		link(from, to, callback) { callback(goError(new FS.ErrnoError(52))); },
		// There's one user, so ownership changes succeed without doing anything.
		chown(path, uid, gid, callback) { callback(null); },
		fchown(fd, uid, gid, callback) { callback(null); },
		lchown(path, uid, gid, callback) { callback(null); },
	};
	globalThis.process.cwd = () => FS.cwd();
	globalThis.process.chdir = (path) => {
		try {
			FS.chdir(path);
		} catch (err) {
			throw goError(err);
		}
	};
}
