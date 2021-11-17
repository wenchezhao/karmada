package explorer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"

	configv1alpha1 "github.com/karmada-io/karmada/pkg/apis/config/v1alpha1"
)

var admissionScheme = runtime.NewScheme()
var admissionCodecs = serializer.NewCodecFactory(admissionScheme)

// ServeHTTP write reply headers and data to the ResponseWriter and then return.
func (wh *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body []byte
	var err error
	ctx := r.Context()

	var reviewResponse Response
	if r.Body == nil {
		err = errors.New("request body is empty")
		klog.Errorf("bad request: %w", err)
		reviewResponse = Errored(http.StatusBadRequest, err)
		wh.writeResponse(w, reviewResponse)
		return
	}

	defer r.Body.Close()
	if body, err = ioutil.ReadAll(r.Body); err != nil {
		klog.Errorf("unable to read the body from the incoming request: %w", err)
		reviewResponse = Errored(http.StatusBadRequest, err)
		wh.writeResponse(w, reviewResponse)
		return
	}

	// verify the content type is accurate
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		err = fmt.Errorf("contentType=%s, expected application/json", contentType)
		klog.Errorf("unable to process a request with an unknown content type: %w", err)
		reviewResponse = Errored(http.StatusBadRequest, err)
		wh.writeResponse(w, reviewResponse)
		return
	}

	request := Request{}
	er := configv1alpha1.ExploreReview{}
	// avoid an extra copy
	er.Request = &request.ExploreRequest
	_, _, err = admissionCodecs.UniversalDeserializer().Decode(body, nil, &er)
	if err != nil {
		klog.Errorf("unable to decode the request: %w", err)
		reviewResponse = Errored(http.StatusBadRequest, err)
		wh.writeResponse(w, reviewResponse)
		return
	}
	klog.V(1).Infof("received request UID: %q, kind: %s", request.UID, request.Kind)

	reviewResponse = wh.Handle(ctx, request)
	wh.writeResponse(w, reviewResponse)
}

// writeResponse writes response to w generically, i.e. without encoding GVK information.
func (wh *Webhook) writeResponse(w io.Writer, response Response) {
	wh.writeExploreResponse(w, configv1alpha1.ExploreReview{
		Response: &response.ExploreResponse,
	})
}

// writeExploreResponse writes ar to w.
func (wh *Webhook) writeExploreResponse(w io.Writer, review configv1alpha1.ExploreReview) {
	if err := json.NewEncoder(w).Encode(review); err != nil {
		klog.Errorf("unable to encode the response: %w", err)
		wh.writeResponse(w, Errored(http.StatusInternalServerError, err))
	} else {
		response := review.Response
		if response.Successful {
			klog.V(4).Infof("wrote response UID: %q, successful: %t", response.UID, response.Successful)
		} else {
			klog.V(4).Infof("wrote response UID: %q, successful: %t, response.status.code: %d, response.status.message: %s",
				response.UID, response.Successful, response.Status.Code, response.Status.Message)
		}
	}
}