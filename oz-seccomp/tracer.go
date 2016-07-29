package seccomp

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path"
	"syscall"

	"golang.org/x/sys/unix"

//	cseccomp "github.com/twtiger/gosecco/constants"
	constants "github.com/shw700/constants"

	"github.com/subgraph/oz"
	"github.com/subgraph/oz/fs"
)

// #include "sys/ptrace.h"
import "C"

const (
	STRINGARG = iota + 1
	PTRARG
	INTARG
)

const (
	SYSCALL_MAP_ARG0_ISMASK = 1
	SYSCALL_MAP_ARG1_ISMASK = (1 << 1)
	SYSCALL_MAP_ARG2_ISMASK = (1 << 2)
	SYSCALL_MAP_ARG3_ISMASK = (1 << 3)
)

type SystemCallArgs []int

type SyscallMapper struct {
	SyscallName string
	Flags uint
	Arg0Class string
	Arg1Class string
	Arg2Class string
	Arg3Class string
}

type SyscallTracker struct {
	scno uint
	rmask uint
	nhits uint
	r0 uint
	r1 uint
	r2 uint
	r3 uint
	r4 uint
	r5 uint
}

var SyscallsTracked = make([]SyscallTracker, 0)

var ( SyscallMappings = []SyscallMapper {
	{ SyscallName: "fcntl",		Arg1Class: "F_" },
	{ SyscallName: "socket",	Arg0Class: "socket_family",	Arg1Class: "socket_type",	Arg2Class: "ip_proto",
		Flags: SYSCALL_MAP_ARG1_ISMASK },
	{ SyscallName: "setsockopt",	Arg1Class: "setsockopt_level",	Arg2Class: "setsockopt_optname" },
	{ SyscallName: "prctl",		Arg0Class: "PR_" },
	{ SyscallName: "mmap",		Arg2Class: "mmap_prot",		Arg3Class: "mmap_flags",
		Flags: SYSCALL_MAP_ARG2_ISMASK|SYSCALL_MAP_ARG3_ISMASK },
	{ SyscallName: "mprotect",	Arg2Class: "mmap_prot",
		Flags: SYSCALL_MAP_ARG2_ISMASK },
	{ SyscallName: "ioctl",		Arg1Class: "ioctl_code" } }
)

func getSyscallTrackerRegVal(st SyscallTracker, rno uint) (uint) {

	switch(rno) {
		case 0:
			return st.r0
		case 1:
			return st.r1
		case 2:
			return st.r2
		case 3:
			return st.r3
		case 4:
			return st.r4
		case 5:
			return st.r5
	}

	return 0
}

func cmpSyscallTracker(st1 SyscallTracker, st2 SyscallTracker) (int) {

	if (st1.scno > st2.scno) {
		return 1
	} else if (st1.scno < st2.scno) {
		return -1
	}

	var i uint = 0

	for i = 0; i < 6; i++ {
		bitmask := uint(0x1 << uint(i))
		var v1 uint = 0
		var v2 uint = 0

		if (st1.rmask & bitmask == 0) && (st2.rmask & bitmask == 0) {
			continue
		}

		if st1.rmask & bitmask > 0 {
			v1 = getSyscallTrackerRegVal(st1, i)
		}

		if st2.rmask & bitmask > 0 {
			v2 = getSyscallTrackerRegVal(st2, i)
		}

		if v1 > v2 {
			return 1
		} else if v1 < v2 {
			return -1
		}

	}

	return 0
}

func getSyscallsTracked() (string) {
	ruleString := ""

	for i := 0; i < len(SyscallsTracked); i++ {
		scn, _ := syscallByNum(int(SyscallsTracked[i].scno))

		var j uint = 0
		first := 1

		// If we're a new syscall, print the name.
		if (i == 0) || (SyscallsTracked[i].scno != SyscallsTracked[i-1].scno) {
			ruleString += scn.name + ": "

			// If we're not the only reference to that syscall number then open a complex expression
			if (i < len(SyscallsTracked) - 1) && (SyscallsTracked[i+1].scno == SyscallsTracked[i].scno ) {
				ruleString += "("
			}

		} else if SyscallsTracked[i].scno == SyscallsTracked[i-1].scno {
			ruleString += "("
		}

		for j = 0; j < 6; j++ {

			if SyscallsTracked[i].rmask & (1 << j) > 0 {
				var valArr = []uint { 0 }
				valArr[0] = getSyscallTrackerRegVal(SyscallsTracked[i], j)
				ruleStr := genArgs(scn.name, j, valArr)

				if first == 0 {
					ruleString += " && "
				} else {
					first = 0
				}

				ruleString += ruleStr
			}

		}

		closed := 0

		if (i > 0) && (SyscallsTracked[i].scno == SyscallsTracked[i-1].scno) {
			ruleString += ")"
			closed = 1
		}

		if (i < len(SyscallsTracked) - 1) && (SyscallsTracked[i+1].scno == SyscallsTracked[i].scno ) {

			if (closed == 0) {
				ruleString += ")"
				closed = 1
			}

			ruleString += " || "
		}


		if (i < len(SyscallsTracked) - 1) && (SyscallsTracked[i+1].scno != SyscallsTracked[i].scno ) {
			ruleString += "\n"
		}

	}

	ruleString += "\n"

	return ruleString
}

func trackSyscall(scno uint, rmask uint, r0 uint, r1 uint, r2 uint, r3 uint, r4 uint, r5 uint) {

	var trackData = SyscallTracker { scno, rmask, 1, r0, r1, r2, r3, r4, r5 }

	if len(SyscallsTracked) == 0 {
		SyscallsTracked = append(SyscallsTracked, trackData)
		return
	}

	// Might not be necessary but let's just leave out the untracked fields.
	if rmask & 1 == 0 {
		trackData.r0 = 0
	}

	if rmask & (1 << 1) == 0 {
		trackData.r1 = 0
	}

	if rmask & (1 << 2) == 0 {
		trackData.r2 = 0
	}

	if rmask & (1 << 3) == 0 {
		trackData.r3 = 0
	}

	if rmask & (1 << 4) == 0 {
		trackData.r4 = 0
	}

	if rmask & (1 << 5) == 0 {
		trackData.r5 = 0
	}

	for i := 0; i <  len(SyscallsTracked); i++ {
		scEq := cmpSyscallTracker(trackData, SyscallsTracked[i])

		if scEq == 0 {
			SyscallsTracked[i].nhits++
			return
		} else if scEq > 0 {
			continue
		}

		SyscallsTracked = append(SyscallsTracked, trackData)
		copy(SyscallsTracked[i+1:], SyscallsTracked[i:])
		SyscallsTracked[i] = trackData
		return
	}

	SyscallsTracked = append(SyscallsTracked, trackData)
	return
}

// Get a constant name that corresponds to a given value paramVal when
// passed as the value of syscall argument argNo for the specified system call.
func getConstNameByCall(syscallName string, paramVal uint, argNo uint) (string) {

	if (argNo > 3) {
		return fmt.Sprint(paramVal)
	}

	for i := 0; i < len(SyscallMappings); i++ {

		if (SyscallMappings[i].SyscallName != syscallName) {
			continue
		}

		argPrefix := SyscallMappings[i].Arg0Class
		lookupMask := 0

		switch (argNo) {
			case 0:
				argPrefix = SyscallMappings[i].Arg0Class

				if SyscallMappings[i].Flags & SYSCALL_MAP_ARG0_ISMASK == SYSCALL_MAP_ARG0_ISMASK {
					lookupMask = 1
				}
			case 1:
				argPrefix = SyscallMappings[i].Arg1Class

				if SyscallMappings[i].Flags & SYSCALL_MAP_ARG1_ISMASK == SYSCALL_MAP_ARG1_ISMASK {
					lookupMask = 1
				}
			case 2:
				argPrefix = SyscallMappings[i].Arg2Class

				if SyscallMappings[i].Flags & SYSCALL_MAP_ARG2_ISMASK == SYSCALL_MAP_ARG2_ISMASK {
					lookupMask = 1
				}
			case 3:
				argPrefix = SyscallMappings[i].Arg3Class

				if SyscallMappings[i].Flags & SYSCALL_MAP_ARG3_ISMASK == SYSCALL_MAP_ARG3_ISMASK {
					lookupMask = 1
				}
		}

		if len(argPrefix) == 0 {
			return fmt.Sprint(paramVal)
		}

		res := ""
		err := error(nil)

		if lookupMask == 0 {
			res, err = constants.GetConstByNo(argPrefix, paramVal)
		} else {
			res, err = constants.GetConstByBitmask(argPrefix, paramVal)
		}

		if err != nil || len(res) == 0 {
			return fmt.Sprint(paramVal)
		}

		return res
	}

	return fmt.Sprint(paramVal)
}

func Tracer() {
	var train = false
	var cmd string
	var cmdArgs []string
	var p *oz.Profile

	var noprofile = flag.Bool("train", false, "Training mode")
	var debug = flag.Bool("debug", false, "Debug")
	var appendpolicy = flag.Bool("append", false, "Append to existing policy if exists")
	var verbosetrain = flag.Bool("verbosetrain", false, "Verbose training output")
	var trainingoutput = flag.String("output", "", "Training policy output file")

	flag.Parse()

	var args = flag.Args()

	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	_, err := os.Stat(args[0])

	if err != nil {
		log.Error("Error: could not access program: ", err)
		os.Exit(-1)
	}

	OzConfig, err := oz.LoadConfig(oz.DefaultConfigPath)
	if err != nil {
		log.Error("unable to load oz config")
		os.Exit(1)
	}

	if *noprofile == true {
		train = true

		// TODO: remove hardcoded path and read prefix from /etc/oz.conf

		cmd = path.Join(OzConfig.PrefixPath, "bin", "oz-seccomp")
		cmdArgs = append([]string{"-mode=train"}, args...)
	} else {
		p = new(oz.Profile)
		if err := json.NewDecoder(os.Stdin).Decode(&p); err != nil {
			log.Error("unable to decode profile data: %v", err)
			os.Exit(1)
		}
		if p.Seccomp.Mode == oz.PROFILE_SECCOMP_TRAIN {
			train = true
		}
		*debug = p.Seccomp.Debug
		cmd = args[0]
		cmdArgs = args[1:]
	}

	var cpid = 0
	done := false

	log.Info("Tracer running command (%v) arguments (%v)\n", cmd, cmdArgs)
	c := exec.Command(cmd)
	c.SysProcAttr = &syscall.SysProcAttr{Ptrace: true}
	c.Env = os.Environ()
	c.Args = append(c.Args, cmdArgs...)

	if *noprofile == false {

		pi, err := c.StdinPipe()
		if err != nil {
			fmt.Errorf("error creating stdin pipe for tracer process: %v", err)
			os.Exit(1)
		}
		jdata, err := json.Marshal(p)
		if err != nil {
			fmt.Errorf("Unable to marshal seccomp state: %+v", err)
			os.Exit(1)
		}
		io.Copy(pi, bytes.NewBuffer(jdata))
		pi.Close()
	}
	children := make(map[int]bool)
	renderFunctions := getRenderingFunctions()

	trainingset := make(map[int]bool)
	freqcount := make(map[int]int)
	trainingargs := make(map[int]map[int][]uint)

	if err := c.Start(); err == nil {
		cpid = c.Process.Pid
		children[c.Process.Pid] = true
		var s syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &s, syscall.WALL, nil)
		children[pid] = true
		if err != nil {
			log.Error("Error (wait4) err:%v pid:%i", err, pid)
		}
		log.Info("Tracing child pid: %v\n", pid)
		pflags := unix.PTRACE_O_TRACESECCOMP
		pflags |= unix.PTRACE_O_TRACEFORK
		pflags |= unix.PTRACE_O_TRACEVFORK
		pflags |= unix.PTRACE_O_TRACECLONE
		pflags |= C.PTRACE_O_EXITKILL
		syscall.PtraceSetOptions(pid, pflags)

		for done == false {
			syscall.PtraceCont(pid, 0)
			pid, err = syscall.Wait4(-1, &s, syscall.WALL, nil)
			if err != nil {
				log.Error("Error (wait4) err:%v pid:%i children:%v\n", err, pid, children)
				done = true
				continue
			}
			children[pid] = true
			if s.Exited() == true {
				delete(children, pid)
				log.Info("Child pid %v finished.\n", pid)
				if len(children) == 0 {
					done = true
				}
				continue
			}
			if s.Signaled() == true {
				log.Error("Pid signaled, pid: %v signal: %v", pid, s)
				delete(children, pid)
				continue
			}
			switch uint32(s) >> 8 {

			case uint32(unix.SIGTRAP) | (unix.PTRACE_EVENT_SECCOMP << 8):
				var regs syscall.PtraceRegs
				err = syscall.PtraceGetRegs(pid, &regs)

				if err != nil {
					log.Error("Error (ptrace): %v", err)
				}

				systemcall, err := syscallByNum(getSyscallNumber(regs))
				if err != nil {
					log.Error("Error: %v", err)
					continue
				}

				/* Render the system call invocation */

				r := getSyscallRegisterArgs(regs)
				call := ""

				if train == true {
					trainingset[getSyscallNumber(regs)] = true
					freqcount[getSyscallNumber(regs)]++
					if systemcall.captureArgs != nil {
						r0 := uint(r[0])
						r1 := uint(r[1])
						r2 := uint(r[2])
						r3 := uint(r[3])
						r4 := uint(r[4])
						r5 := uint(r[5])
						rmask := uint(0)

						for c, i := range systemcall.captureArgs {
							if i == 1 {
							rmask |= (uint(1) << uint(c))
								if trainingargs[getSyscallNumber(regs)] == nil {
									trainingargs[getSyscallNumber(regs)] = make(map[int][]uint)
								}
								if contains(trainingargs[getSyscallNumber(regs)][c], uint(r[c])) == false {
									trainingargs[getSyscallNumber(regs)][c] = append(trainingargs[getSyscallNumber(regs)][c], uint(r[c]))
								}
							}
						}

						trackSyscall(uint(getSyscallNumber(regs)), rmask, r0, r1, r2, r3, r4, r5)
					}
				}

				if f, ok := renderFunctions[getSyscallNumber(regs)]; ok {
					call, err = f(pid, r)
					if err != nil {
						log.Info("%v", err)
						continue
					}
					if *debug == true {
						call += "\n  " + renderSyscallBasic(pid, systemcall, regs)
					}
				} else {
					call = renderSyscallBasic(pid, systemcall, regs)
				}

				log.Info("seccomp hit on sandbox pid %v (%v) syscall %v (%v):\n  %s", pid, getProcessCmdLine(pid), systemcall.name, systemcall.num, call)
				continue

			case uint32(unix.SIGTRAP) | (unix.PTRACE_EVENT_EXIT << 8):
				if *debug == true {
					log.Error("Ptrace exit event detected pid %v (%s)", pid, getProcessCmdLine(pid))
				}
				delete(children, pid)
				continue

			case uint32(unix.SIGTRAP) | (unix.PTRACE_EVENT_CLONE << 8):
				newpid, err := syscall.PtraceGetEventMsg(pid)
				if err != nil {
					log.Error("PTrace event message retrieval failed: %v", err)
				}
				children[int(newpid)] = true
				if *debug == true {
					log.Error("Ptrace clone event detected pid %v (%s)", pid, getProcessCmdLine(pid))
				}
				continue
			case uint32(unix.SIGTRAP) | (unix.PTRACE_EVENT_FORK << 8):
				if *debug == true {
					log.Error("PTrace fork event detected pid %v (%s)", pid, getProcessCmdLine(pid))
				}
				newpid, err := syscall.PtraceGetEventMsg(pid)
				if err != nil {
					log.Error("PTrace event message retrieval failed: %v", err)
				}
				children[int(newpid)] = true
				continue
			case uint32(unix.SIGTRAP) | (unix.PTRACE_EVENT_VFORK << 8):
				if *debug == true {
					log.Error("Ptrace vfork event detected pid %v (%s)", pid, getProcessCmdLine(pid))
				}
				newpid, err := syscall.PtraceGetEventMsg(pid)
				if err != nil {
					log.Error("PTrace event message retrieval failed: %v", err)
				}
				children[int(newpid)] = true
				continue
			case uint32(unix.SIGTRAP) | (unix.PTRACE_EVENT_VFORK_DONE << 8):
				if *debug == true {
					log.Error("Ptrace vfork done event detected pid %v (%s)", pid, getProcessCmdLine(pid))
				}
				newpid, err := syscall.PtraceGetEventMsg(pid)
				if err != nil {
					log.Error("PTrace event message retrieval failed: %v", err)
				}
				children[int(newpid)] = true

				continue
			case uint32(unix.SIGTRAP) | (unix.PTRACE_EVENT_EXEC << 8):
				if *debug == true {
					log.Error("Ptrace exec event detected pid %v (%s)", pid, getProcessCmdLine(pid))
				}
				continue
			case uint32(unix.SIGTRAP) | (unix.PTRACE_EVENT_STOP << 8):
				if *debug == true {
					log.Error("Ptrace stop event detected pid %v (%s)", pid, getProcessCmdLine(pid))
				}
				continue
			case uint32(unix.SIGTRAP):
				if *debug == true {
					log.Error("SIGTRAP detected in pid %v (%s)", pid, getProcessCmdLine(pid))
				}
				continue
			case uint32(unix.SIGCHLD):
				if *debug == true {
					log.Error("SIGCHLD detected pid %v (%s)", pid, getProcessCmdLine(pid))
				}
				continue
			case uint32(unix.SIGSTOP):
				if *debug == true {
					log.Error("SIGSTOP detected pid %v (%s)", pid, getProcessCmdLine(pid))
				}
				continue
			case uint32(unix.SIGSEGV):
				if *debug == true {
					log.Error("SIGSEGV detected pid %v (%s)", pid, getProcessCmdLine(pid))
				}
				err = syscall.Kill(pid, 9)
				if err != nil {
					log.Error("kill: %v", err)
					os.Exit(1)
				}
				delete(children, pid)
				continue
			default:
				y := s.StopSignal()
				if *debug == true {
					log.Error("Child stopped for unknown reasons pid %v status %v signal %i (%s)", pid, s, y, getProcessCmdLine(pid))
				}
				continue
			}
		}

		if train == true {
			var u *user.User
			var e error
			u, e = user.Current()
			var resolvedpath = ""

			if e != nil {
				log.Error("user.Current(): %v", e)
			}

			if *trainingoutput != "" {
				resolvedpath = *trainingoutput
			} else {
				if *noprofile == false {
					resolvedpath, e = fs.ResolvePathNoGlob(p.Seccomp.TrainOutput, -1, u, nil)
					if e != nil {
						log.Error("resolveVars(): %v", e)
					}
				} else {
					s := fmt.Sprintf("${HOME}/%s-%d.seccomp", fname(os.Args[2]), cpid)
					resolvedpath, e = fs.ResolvePathNoGlob(s, -1, u, nil)
				}
			}
			policyout := ""
			sk := sortedKeys(freqcount)
			if *verbosetrain == true {
				fmt.Println("\nInvocation counts for observed system calls:\n")
			}
			for _, call := range sk {
				sc, _ := syscallByNum(call)
				if *verbosetrain == true {
					fmt.Printf("%s calls: %d\n", sc.name, freqcount[call])
				}
				done := false
				for c := range trainingargs {
					if c == call {
						done = true
					}
				}
				if done == false {
					sc, _ := syscallByNum(call)
					policyout += fmt.Sprintf("%s:1\n", sc.name)
				}
			}
			policyout += getSyscallsTracked()
			policyout += fmt.Sprintf("execve:1")
			if *verbosetrain == true {
				fmt.Println("\nTrainer generated seccomp-bpf whitelist policy:\n")
				fmt.Println(policyout)
			}
			if *appendpolicy == true {
				log.Error("Not yet implemented.")
			}

			f, err := os.OpenFile(resolvedpath, os.O_CREATE|os.O_RDWR, 0600)
			if err == nil {
				_, err := f.WriteString(policyout)
				if err != nil {
					log.Error("Error writing policy file: %v", err)
				}
				err = f.Close()
				if err != nil {
					log.Error("Error closing policy file: %v", err)
				}
			} else {
				log.Error("Error opening policy file \"%s\": %v", resolvedpath, err)
			}
		}
	}
}

func genArgs(scName string, a uint, vals []uint) string {
	s := ""
	for idx, x := range vals {
		//s += fmt.Sprintf(" arg%d == %d ", a, x)
		s += fmt.Sprintf("arg%d == %s", a, getConstNameByCall(scName, x, a))

		if idx < len(vals)-1 {
			s += "||"
		}
	}
	return s
}

func contains(slice []uint, val uint) bool {
	for _, x := range slice {
		if val == x {
			return true
		}
	}
	return false
}
func fname(p string) string {
	_, fname := path.Split(p)
	return fname
}
