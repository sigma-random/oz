package seccomp

import (
	"fmt"
	"os"
)

// #include "asm-generic/ioctl.h"
// #include "asm-generic/ioctls.h"
import "C"

var ioctls = map[uint]string{

	// 0x54

	C.TCGETS:       "TCGETS",
	C.TCSETS:       "TCSETS",
	C.TCSETSW:      "TCSETSW",
	C.TCGETA:       "TCGETA",
	C.TCSETAW:      "TCSETAW",
	C.TCSETAF:      "TCSETAF",
	C.TCSBRK:       "TCSBRK",
	C.TCXONC:       "TCXONC",
	C.TCFLSH:       "TCFLSH",
	C.TIOCEXCL:     "TIOCEXCL",
	C.TIOCNXCL:     "TIOCNXCL",
	C.TIOCSCTTY:    "TIOCSCTTY",
	C.TIOCGPGRP:    "TIOCGPGRP",
	C.TIOCSPGRP:    "TIOCSPGRP",
	C.TIOCOUTQ:     "TIOCOUTQ",
	C.TIOCSTI:      "TIOCSTI",
	C.TIOCGWINSZ:   "TIOCGWINSZ",
	C.TIOCSWINSZ:   "TIOCSWINSZ",
	C.TIOCMGET:     "TIOCMGET",
	C.TIOCMBIS:     "TIOCMBIS",
	C.TIOCMBIC:     "TIOCMBIC",
	C.TIOCMSET:     "TIOCMSET",
	C.TIOCGSOFTCAR: "TIOCGSOFTCAR",
	C.TIOCSSOFTCAR: "TIOCSSOFTCAR",
	C.FIONREAD:     "FIONREAD",
	// C.TIOCINQ: FIONREAD (TBD)
	C.TIOCLINUX:   "TIOCLINUX",
	C.TIOCCONS:    "TIOCCONS",
	C.TIOCGSERIAL: "TIOCGSERIAL",
	C.TIOCSSERIAL: "TIOCSSERIAL",
	C.TIOCPKT:     "TIOCPKT",
	C.FIONBIO:     "FIONBIO",
	C.TIOCNOTTY:   "TIOCNOTTY",
	C.TIOCSETD:    "TIOCSETD",
	C.TIOCGETD:    "TIOCGETD",
	C.TCSBRKP:     "TCSBRKP",
	C.TIOCSBRK:    "TIOCSBRK",
	C.TIOCCBRK:    "TIOCCBRK",
	C.TIOCGSID:    "TIOCGSID",
	// C.TCGETS2: TBD
	// C.TCSETS2: TBD
	// C.TCSETSW2: TBD
	// C.TCSETSF2: TBD
	C.TIOCGRS485: "TIOCGRS485",
	C.TIOCSRS485: "TIOCSRS485",
	// C.TIOCGPTN: TBD
	// C.TIOCSPTLCK: TBD
	// C.TIOCGDEV: TBD
	C.TCGETX:  "TCGETX",
	C.TCSETX:  "TCSETX",
	C.TCSETXF: "TCSETXF",
	C.TCSETXW: "TCSETXW",
	// C.TIOCSIG:
	C.TIOCVHANGUP: "TIOCVHANGUP",
	// C.TIOCGPKT:
	// C.TIOCGPTLCK:
	// C.TIOCGEXCL:
	C.FIONCLEX:        "FIONCLEX",
	C.FIOCLEX:         "FIOCLEX",
	C.FIOASYNC:        "FIOASYNC",
	C.TIOCSERCONFIG:   "TIOCSERCONFIG",
	C.TIOCSERGWILD:    "TIOCSERGWILD",
	C.TIOCSERSWILD:    "TIOCSERSWILD",
	C.TIOCGLCKTRMIOS:  "TIOCGLCKTRMIOS",
	C.TIOCSLCKTRMIOS:  "TIOCSLCKTRMIOS",
	C.TIOCSERGSTRUCT:  "TIOCSERGSTRUCT",
	C.TIOCSERGETLSR:   "TIOCSERGETLSR",
	C.TIOCSERGETMULTI: "TIOCSERGETMULTI",
	C.TIOCSERSETMULTI: "TIOCSERSETMULTI",
	C.TIOCMIWAIT:      "TIOCMIWAIT",
	C.TIOCGICOUNT:     "TIOCGICOUNT",
	C.FIOQSIZE:        "FIOQSIZE",
}

func render_ioctl(pid int, args RegisterArgs) (string, error) {

	arg1 := uint(args[1])

	dir := (arg1 >> C._IOC_DIRSHIFT) & C._IOC_DIRMASK
	t := (arg1 >> C._IOC_TYPESHIFT) & C._IOC_TYPEMASK
	nr := (arg1 >> C._IOC_NRSHIFT) & C._IOC_NRMASK // _IOC_NRSHIFT = 0
	size := (arg1 >> C._IOC_SIZESHIFT) & C._IOC_SIZEMASK

	/* TODO: Move this into procsnitch */

	procpath := fmt.Sprintf("/proc/%d/fd/%d", pid, uint(args[0]))
	str, err := os.Readlink(procpath)
	if err != nil {
		fmt.Printf("Error! %v", err)
		return "", nil
	}

	/* End */

	ioctlstr := ioctls[uint(args[1])]
	if ioctlstr == "" {
		ioctlstr = fmt.Sprintf("{ dir=%x type=%c number=%x size=%v }", dir, byte(t), nr, size)
	}
	callrep := fmt.Sprintf("ioctl(%d (%s), %s, 0x%X)", args[0], str, ioctlstr, args[2])
	return callrep, nil
}
