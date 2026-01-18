//go:build pam && linux && cgo

package auth

/*
#cgo LDFLAGS: -lpam
#include <stdlib.h>
#include <security/pam_appl.h>

// Simple PAM conversation function that supplies a single password.
static int kiki_conv(int num_msg, const struct pam_message **msg,
                     struct pam_response **resp, void *appdata_ptr) {
  if (num_msg <= 0) return PAM_CONV_ERR;
  struct pam_response *replies = (struct pam_response*)calloc(num_msg, sizeof(struct pam_response));
  if (replies == NULL) return PAM_CONV_ERR;

  const char* pw = (const char*)appdata_ptr;
  for (int i = 0; i < num_msg; i++) {
    // Provide password for prompts; ignore others.
    if (msg[i]->msg_style == PAM_PROMPT_ECHO_OFF || msg[i]->msg_style == PAM_PROMPT_ECHO_ON) {
      replies[i].resp = strdup(pw != NULL ? pw : "");
    }
  }
  *resp = replies;
  return PAM_SUCCESS;
}

static int kiki_pam_auth(const char* service, const char* user, const char* pw) {
  struct pam_conv conv;
  conv.conv = kiki_conv;
  conv.appdata_ptr = (void*)pw;

  pam_handle_t* pamh = NULL;
  int ret = pam_start(service, user, &conv, &pamh);
  if (ret != PAM_SUCCESS) return ret;

  ret = pam_authenticate(pamh, 0);
  if (ret == PAM_SUCCESS) {
    ret = pam_acct_mgmt(pamh, 0);
  }
  pam_end(pamh, ret);
  return ret;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Authenticate validates username/password via PAM.
// Service defaults to "login".
func Authenticate(username, password string) error {
	if username == "" {
		return errors.New("empty username")
	}
	csrv := C.CString("login")
	cu := C.CString(username)
	cpw := C.CString(password)
	defer C.free(unsafe.Pointer(csrv))
	defer C.free(unsafe.Pointer(cu))
	defer C.free(unsafe.Pointer(cpw))

	ret := C.kiki_pam_auth(csrv, cu, cpw)
	if ret != C.PAM_SUCCESS {
		return errors.New("authentication failed")
	}
	return nil
}
