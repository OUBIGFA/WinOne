//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"winone/internal/configjson"
	"winone/internal/directorylinks"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/processidentity"
	"winone/internal/transport"
)

// registerDirectoryLinks adds a nonresident tool; history is local machine
// state, outside portable settings and the optional-module inventory.
func registerDirectoryLinks(service *host.Service, dataDirectory string) error {
	m, err := directorylinks.New(filepath.Join(dataDirectory, "tools", "directory-links"))
	if err != nil {
		// A damaged tool history must remain visible without preventing the
		// application's diagnostics and unrelated tools from opening.
		failure := err.Error()
		service.RegisterModule("directory-links", func() host.Module {
			return host.Module{ID: "directory-links", Name: "文件与目录链接", State: "faulted", Detail: failure}
		}, func(r host.Request) host.Response {
			return host.Response{ID: r.ID, Error: &host.RPCError{Code: "directory_links_history", Message: failure}}
		})
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	authorize := func(pid uint32) error {
		process, err := processidentity.Open(pid, filepath.Join(filepath.Dir(executable), "WinOne.exe"))
		if err != nil {
			return err
		}
		process.Close()
		return nil
	}
	service.RegisterModule("directory-links", func() host.Module {
		return host.Module{ID: "directory-links", Name: "文件与目录链接", State: "ready"}
	}, directoryLinksHandler(m, authorize))
	return nil
}

func directoryLinksHandler(m *directorylinks.Manager, authorize func(uint32) error) func(host.Request) host.Response {
	var mu sync.Mutex
	owners := map[string]uint32{}
	pendingPaths := []string{}
	pendingRequested := false
	var pendingOptions *directoryLinkLaunchOptions
	var scanCancel context.CancelFunc
	var scanOwner uint32
	var scanSequence uint64
	return func(request host.Request) host.Response {
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "directory_links", Message: err.Error()}}
		}
		// Only activation data is accepted from another same-user launcher. An
		// elevated Host must not become an arbitrary client's filesystem proxy.
		if request.Method != "open" && request.Method != "pending" {
			if authorize == nil {
				return fail(errors.New("链接操作身份验证未配置"))
			}
			if err := authorize(request.PeerPID); err != nil {
				return fail(err)
			}
		}
		var result any
		var err error
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		switch request.Method {
		case "open":
			var args struct {
				Paths   []string                    `json:"paths"`
				Options *directoryLinkLaunchOptions `json:"options,omitempty"`
			}
			if err = configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			if len(args.Paths) > 200 {
				return fail(errors.New("每次预填最多 200 个来源"))
			}
			if args.Options != nil && args.Options.Kind != "" && args.Options.Kind != "auto" && args.Options.Kind != "hardlink" && args.Options.Kind != "junction" && args.Options.Kind != "symlink" {
				return fail(errors.New("预填链接类型无效"))
			}
			for _, path := range args.Paths {
				if !filepath.IsAbs(path) || len(path) > 32760 || strings.ContainsAny(path, "\x00\r\n") {
					return fail(errors.New("预填来源必须是有效绝对路径"))
				}
			}
			mu.Lock()
			if len(pendingPaths)+len(args.Paths) > 200 {
				mu.Unlock()
				return fail(errors.New("待处理来源过多，请先处理当前列表"))
			}
			pendingPaths = append(pendingPaths, args.Paths...)
			pendingRequested = true
			if args.Options != nil {
				copy := *args.Options
				pendingOptions = &copy
			}
			mu.Unlock()
			result = map[string]bool{"queued": true}
		case "pending":
			mu.Lock()
			paths := append([]string{}, pendingPaths...)
			pendingPaths = nil
			options := pendingOptions
			pendingOptions = nil
			requested := pendingRequested
			pendingRequested = false
			mu.Unlock()
			value := map[string]any{"requested": requested, "paths": paths}
			if options != nil {
				value["options"] = options
			}
			result = value
		case "list":
			result = m.List()
		case "browse":
			var args struct {
				Directory string `json:"directory"`
				Offset    int    `json:"offset"`
			}
			if err = configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			result, err = m.Browse(ctx, args.Directory, args.Offset)
		case "menu.get", "menu.set":
			exe, e := os.Executable()
			if e != nil {
				return fail(e)
			}
			desktop := filepath.Join(filepath.Dir(exe), "WinOne.exe")
			if request.Method == "menu.get" {
				result, err = directorylinks.ExplorerMenu(desktop)
			} else {
				var args struct {
					Enabled bool `json:"enabled"`
				}
				if err = configjson.DecodeObject(request.Params, &args); err != nil {
					return fail(err)
				}
				result, err = directorylinks.ConfigureExplorerMenu(desktop, args.Enabled)
			}
		case "scan":
			var args struct {
				Directory string `json:"directory"`
				Recursive bool   `json:"recursive"`
			}
			if err = configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			scanCtx, stop := context.WithCancel(ctx)
			mu.Lock()
			scanSequence++
			sequence := scanSequence
			scanCancel = stop
			scanOwner = request.PeerPID
			mu.Unlock()
			defer func() {
				stop()
				mu.Lock()
				if scanSequence == sequence {
					scanCancel = nil
				}
				mu.Unlock()
			}()
			result, err = m.Scan(scanCtx, args.Directory, args.Recursive)
		case "scan.cancel":
			mu.Lock()
			if scanCancel != nil && scanOwner == request.PeerPID {
				scanCancel()
			}
			mu.Unlock()
			result = map[string]bool{"requested": true}
		case "reveal":
			var args struct {
				ID   string `json:"id"`
				Side string `json:"side"`
			}
			if err = configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			err = m.Reveal(args.ID, args.Side)
			result = map[string]bool{"opened": err == nil}
		case "preview":
			var args directorylinks.Request
			if err = configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			var p directorylinks.Plan
			p, err = m.Preview(args)
			result = p
			if err == nil {
				mu.Lock()
				if len(owners) > 128 {
					owners = map[string]uint32{}
				}
				owners[p.ID] = request.PeerPID
				mu.Unlock()
			}
		case "previewBatch":
			var args directorylinks.BatchRequest
			if err = configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			var p directorylinks.BatchPlan
			p, err = m.PreviewBatch(args)
			result = p
			if err == nil {
				mu.Lock()
				owners[p.ID] = request.PeerPID
				mu.Unlock()
			}
		case "exportPlan":
			var args struct {
				Token     string `json:"token"`
				Directory string `json:"directory"`
			}
			if err = configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			mu.Lock()
			owner, ok := owners[args.Token]
			mu.Unlock()
			if !ok || owner != request.PeerPID {
				return fail(errors.New("请先预览本次计划"))
			}
			var path string
			path, err = m.ExportPlan(args.Token, args.Directory)
			result = map[string]string{"path": path}
		case "create", "create.elevated", "createBatch", "createBatch.elevated":
			var args struct {
				Token     string `json:"token"`
				Confirmed bool   `json:"confirmed"`
			}
			if err = configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			mu.Lock()
			owner, ok := owners[args.Token]
			if ok && owner == request.PeerPID {
				delete(owners, args.Token)
			}
			mu.Unlock()
			if !ok || owner != request.PeerPID {
				return fail(errors.New("预览属于另一窗口或已过期，请重新预览"))
			}
			elevated := strings.HasSuffix(request.Method, ".elevated")
			if elevated && !args.Confirmed {
				return fail(errors.New("请明确选择本次以管理员权限创建"))
			}
			if strings.HasPrefix(request.Method, "createBatch") {
				execute := func(plans []directorylinks.Plan) ([]directorylinks.ExecutionResult, error) {
					return directorylinks.ExecuteBatch(plans), nil
				}
				if elevated {
					execute = func(plans []directorylinks.Plan) ([]directorylinks.ExecutionResult, error) {
						return elevateDirectoryLinks(ctx, plans)
					}
				}
				result, err = m.CreateBatchWith(ctx, args.Token, execute)
			} else if elevated {
				if !args.Confirmed {
					return fail(errors.New("请明确选择本次以管理员权限创建"))
				}
				result, err = m.CreateWith(ctx, args.Token, func(p directorylinks.Plan) error { return elevateDirectoryLink(ctx, p) })
			} else {
				result, err = m.Create(ctx, args.Token)
			}
		case "inspect", "remove", "forget":
			var args struct {
				ID        string `json:"id"`
				Confirmed bool   `json:"confirmed"`
				CheckedAt string `json:"checkedAt,omitempty"`
			}
			if err = configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			if request.Method == "inspect" {
				result, err = m.Check(ctx, args.ID)
			} else {
				if !args.Confirmed {
					return fail(errors.New("请确认移除所选入口或记录"))
				}
				if request.Method == "remove" {
					if args.CheckedAt == "" {
						return fail(errors.New("请重新检查链接后确认移除"))
					}
					result, err = m.Remove(ctx, args.ID, args.CheckedAt)
				} else {
					err = m.Forget(args.ID)
					result = map[string]bool{"forgotten": err == nil}
				}
			}
		case "inspectBatch":
			var args struct {
				IDs []string `json:"ids"`
			}
			if err = configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			result, err = m.CheckMany(ctx, args.IDs)
		default:
			return fail(errors.New("不支持的链接操作"))
		}
		if err != nil {
			return fail(err)
		}
		data, err := json.Marshal(result)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: request.ID, Result: data}
	}
}

type directoryLinkLaunchOptions struct {
	Kind         string `json:"kind,omitempty"`
	Relative     *bool  `json:"relative,omitempty"`
	RetainScript *bool  `json:"retainScript,omitempty"`
	HideSuccess  *bool  `json:"hideSuccess,omitempty"`
}

type directoryElevationExchange struct {
	mu                sync.Mutex
	executable, nonce string
	pid               uint32
	ready             chan struct{}
	ctx               context.Context
	plans             []directorylinks.Plan
	claimed, finished bool
	completion        chan []directorylinks.ExecutionResult
}

func (e *directoryElevationExchange) Handle(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "directory_link_elevation", Message: err.Error()}}
	}
	select {
	case <-e.ready:
	case <-e.ctx.Done():
		return fail(e.ctx.Err())
	}
	var args struct {
		Nonce   string                           `json:"nonce"`
		Error   string                           `json:"error,omitempty"`
		Results []directorylinks.ExecutionResult `json:"results,omitempty"`
	}
	if err := configjson.DecodeObject(r.Params, &args); err != nil {
		return fail(err)
	}
	if r.Version != 1 || r.PeerPID == 0 || r.PeerPID != e.pid || args.Nonce != e.nonce || len(args.Error) > 8192 {
		return fail(errors.New("权限操作身份无效"))
	}
	process, err := processidentity.Open(r.PeerPID, e.executable)
	if err != nil {
		return fail(err)
	}
	defer process.Close()
	if !process.Elevated {
		return fail(errors.New("组件未获得本次管理员权限"))
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if r.Method == "claim" && !e.claimed && args.Error == "" {
		e.claimed = true
		raw, err := json.Marshal(struct {
			Plans []directorylinks.Plan `json:"plans"`
		}{e.plans})
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: r.ID, Result: raw}
	}
	if r.Method == "complete" && e.claimed && !e.finished {
		if len(args.Results) != len(e.plans) {
			return fail(errors.New("权限操作结果数量不匹配"))
		}
		for i, row := range args.Results {
			if row.ID != e.plans[i].ID || len(row.Error) > 8192 {
				return fail(errors.New("权限操作结果与预览不一致"))
			}
		}
		e.finished = true
		e.completion <- args.Results
		return host.Response{ID: r.ID, Result: json.RawMessage(`{"accepted":true}`)}
	}
	return fail(errors.New("权限操作已使用或顺序无效"))
}

// elevateDirectoryLink sends no path or arbitrary command in the elevated
// command line. A same-user, PID-verified private pipe carries exactly one plan.
func elevateDirectoryLink(ctx context.Context, p directorylinks.Plan) error {
	rows, err := elevateDirectoryLinks(ctx, []directorylinks.Plan{p})
	if err != nil {
		return err
	}
	if len(rows) != 1 || !rows[0].Attempted {
		return errors.New("权限组件未执行本次创建")
	}
	if rows[0].Error != "" {
		return errors.New(rows[0].Error)
	}
	return nil
}

func elevateDirectoryLinks(ctx context.Context, plans []directorylinks.Plan) ([]directorylinks.ExecutionResult, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	nonce := rand.Text()
	pipe := `\\.\pipe\WinOne-directory-links-` + nonce
	listener, err := transport.Listen(pipe)
	if err != nil {
		return nil, err
	}
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	exchange := &directoryElevationExchange{executable: executable, nonce: nonce, ready: make(chan struct{}), ctx: childCtx, plans: plans, completion: make(chan []directorylinks.ExecutionResult, 1)}
	served := make(chan error, 1)
	go func() { served <- transport.Serve(childCtx, listener, exchange) }()
	defer func() { cancel(); _ = listener.Close(); <-served }()
	pid, wait, err := modules.LaunchElevated(executable, []string{"--directory-links-elevated", pipe, nonce, strconv.Itoa(os.Getpid())})
	exchange.pid = pid
	close(exchange.ready)
	if err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- wait() }()
	select {
	case result := <-exchange.completion:
		select {
		case ended := <-exited:
			return result, ended
		case <-time.After(10 * time.Second):
			return result, errors.New("权限组件尚未退出，请稍后重新检查链接")
		}
	case ended := <-exited:
		select {
		case result := <-exchange.completion:
			return result, ended
		default:
			return nil, errors.Join(errors.New("权限组件未确认创建结果，请重新检查"), ended)
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// directoryLinksCommand is dispatched before ordinary Host startup. It neither
// reads a profile nor registers startup; only the live parent can supply work.
func directoryLinksCommand(args []string, executable string) (bool, error) {
	if len(args) == 0 || args[0] != "--directory-links-elevated" {
		return false, nil
	}
	if len(args) != 4 || len(args[2]) != 26 || args[1] != `\\.\pipe\WinOne-directory-links-`+args[2] || strings.ContainsAny(args[2], `/\ :`) {
		return true, errors.New("链接权限参数无效")
	}
	parent, err := strconv.ParseUint(args[3], 10, 32)
	if err != nil || parent == 0 {
		return true, errors.New("链接权限父进程无效")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return true, errors.New("链接权限组件未获提升")
	}
	owner, err := processidentity.Open(uint32(parent), executable)
	if err != nil {
		return true, err
	}
	defer owner.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() { _ = owner.Wait(ctx); cancel() }()
	call := func(method string, value any) (host.Response, error) {
		raw, err := json.Marshal(value)
		if err != nil {
			return host.Response{}, err
		}
		response, err := transport.CallWithVerifiedServer(ctx, args[1], host.Request{Version: 1, ID: method, Method: method, Params: raw}, 5*time.Second, func(pid uint32) error {
			if pid != uint32(parent) {
				return errors.New("权限请求来自其他进程")
			}
			process, err := processidentity.Open(pid, executable)
			if err == nil {
				process.Close()
			}
			return err
		})
		if err == nil && response.Error != nil {
			err = errors.New(response.Error.Message)
		}
		return response, err
	}
	response, err := call("claim", map[string]string{"nonce": args[2]})
	if err != nil {
		return true, err
	}
	var work struct {
		Plans []directorylinks.Plan `json:"plans"`
	}
	if err = configjson.DecodeObject(response.Result, &work); err != nil {
		return true, err
	}
	if err = ctx.Err(); err != nil {
		return true, err
	}
	if len(work.Plans) == 0 || len(work.Plans) > 200 {
		return true, errors.New("权限操作数量无效")
	}
	results := directorylinks.ExecuteBatch(work.Plans)
	_, reportErr := call("complete", struct {
		Nonce   string                           `json:"nonce"`
		Results []directorylinks.ExecutionResult `json:"results"`
	}{args[2], results})
	return true, reportErr
}
