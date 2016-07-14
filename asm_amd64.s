#include "textflag.h"

TEXT ·atomicOr32(SB),NOSPLIT,$0-12
	MOVQ addr+0(FP), BP
	MOVL val+8(FP), AX
	LOCK
	ORL AX, 0(BP)
	RET
