package credhub_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"code.cloudfoundry.org/cnbapplifecycle/pkg/credhub"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("credhub", func() {
	Describe("InterpolateServiceRefs", func() {
		var (
			vcapServicesValue        string
			vcapPlatformOptionsValue string
			server                   *ghttp.Server
			err                      error

			maxConnectAttempts int
			retryDelay         time.Duration
		)

		VerifyClientCerts := func() http.HandlerFunc {
			return func(w http.ResponseWriter, req *http.Request) {
				tlsConnectionState := req.TLS
				Expect(tlsConnectionState).NotTo(BeNil())
				Expect(tlsConnectionState.PeerCertificates).NotTo(BeEmpty())
				Expect(tlsConnectionState.PeerCertificates[0].Subject.CommonName).To(Equal("client"))
			}
		}

		BeforeEach(func() {
			server = ghttp.NewUnstartedServer()

			cert, err := tls.LoadX509KeyPair(filepath.Join(certDir, "server.crt"), filepath.Join(certDir, "server.key"))
			Expect(err).NotTo(HaveOccurred())

			caCerts := x509.NewCertPool()

			caCertBytes, err := os.ReadFile(filepath.Join(certDir, "cacerts", "ca.crt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(caCerts.AppendCertsFromPEM(caCertBytes)).To(BeTrue())

			server.HTTPTestServer.TLS = &tls.Config{
				ClientAuth:   tls.RequireAndVerifyClientCert,
				Certificates: []tls.Certificate{cert},
				ClientCAs:    caCerts,
			}
			server.HTTPTestServer.StartTLS()

			GinkgoT().Setenv("CF_INSTANCE_CERT", filepath.Join(certDir, "client.crt"))
			GinkgoT().Setenv("CF_INSTANCE_KEY", filepath.Join(certDir, "client.key"))
			GinkgoT().Setenv("CF_SYSTEM_CERT_PATH", filepath.Join(certDir, "cacerts"))

			maxConnectAttempts = 5
			retryDelay = 0 * time.Second

		})

		AfterEach(func() {
			server.Close()
		})

		BeforeEach(func() {
			vcapServicesValue = `{"my-server":[{"credentials":{"credhub-ref":"(//my-server/creds)"}}]}`
			GinkgoT().Setenv("VCAP_SERVICES", vcapServicesValue)

			vcapPlatformOptionsValue = fmt.Sprintf(`{"credhub-uri": "%s"}`, server.URL())
			GinkgoT().Setenv("VCAP_PLATFORM_OPTIONS", vcapPlatformOptionsValue)
		})

		JustBeforeEach(func() {
			err = credhub.InterpolateServiceRefsFromVcapServices(maxConnectAttempts, retryDelay)
		})

		Context("when there are no credhub refs in VCAP_SERVICES and no TLS environment variables are present", func() {
			BeforeEach(func() {
				os.Unsetenv("CF_INSTANCE_CERT")
				os.Unsetenv("CF_INSTANCE_KEY")
				os.Unsetenv("CF_SYSTEM_CERT_PATH")

				vcapServicesValue = `{"my-server":[{"credentials":{"no refs here":"and this string containing credhub-ref doesnt count"}}]}`
				GinkgoT().Setenv("VCAP_SERVICES", vcapServicesValue)
			})

			It("does not fail and does not change VCAP_SERVICES", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(os.Getenv("VCAP_SERVICES")).To(Equal(vcapServicesValue))
			})
		})

		Context("when credhub successfully interpolates", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/api/v1/interpolate"),
						ghttp.VerifyBody([]byte(vcapServicesValue)),
						VerifyClientCerts(),
						ghttp.RespondWith(http.StatusOK, "INTERPOLATED_JSON"),
					))
			})

			It("updates VCAP_SERVICES with the interpolated content and runs the process without VCAP_PLATFORM_OPTIONS", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(os.Getenv("VCAP_SERVICES")).To(Equal("INTERPOLATED_JSON"))
			})

		})

		Context("when credhub fails initially, but eventually succeeds", func() {
			BeforeEach(func() {

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/api/v1/interpolate"),
						ghttp.VerifyBody([]byte(vcapServicesValue)),
						ghttp.RespondWith(http.StatusInternalServerError, "{}"),
					))

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/api/v1/interpolate"),
						ghttp.VerifyBody([]byte(vcapServicesValue)),
						VerifyClientCerts(),
						ghttp.RespondWith(http.StatusOK, "INTERPOLATED_JSON"),
					))
			})

			It("updates VCAP_SERVICES with the interpolated content and runs the process without VCAP_PLATFORM_OPTIONS", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(os.Getenv("VCAP_SERVICES")).To(Equal("INTERPOLATED_JSON"))
			})
		})

		Context("when credhub always fails", func() {
			BeforeEach(func() {
				for attempt := 1; attempt <= maxConnectAttempts; attempt++ {
					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("POST", "/api/v1/interpolate"),
							ghttp.VerifyBody([]byte(vcapServicesValue)),
							ghttp.RespondWith(http.StatusInternalServerError, "{}"),
						))
				}
			})

			Context("and it never succeeds", func() {
				It("returns an error and doesn't change VCAP_SERVICES", func() {
					Expect(err).To(MatchError(MatchRegexp("unable to interpolate credhub references")))
					Expect(os.Getenv("VCAP_SERVICES")).To(Equal(vcapServicesValue))
				})
			})
		})

		Context("when the instance cert and key are invalid", func() {
			BeforeEach(func() {
				GinkgoT().Setenv("CF_INSTANCE_CERT", filepath.Join(certDir, "not_a_cert"))
				GinkgoT().Setenv("CF_INSTANCE_KEY", filepath.Join(certDir, "not_a_cert"))
			})

			It("returns an error and doesn't change VCAP_SERVICES", func() {
				Expect(err).To(MatchError(MatchRegexp("unable to set up credhub client")))
				Expect(os.Getenv("VCAP_SERVICES")).To(Equal(vcapServicesValue))
			})
		})

		Context("when the instance cert and key aren't set", func() {
			BeforeEach(func() {
				os.Unsetenv("CF_INSTANCE_CERT")
				os.Unsetenv("CF_INSTANCE_KEY")
			})

			It("returns an error and doesn't change VCAP_SERVICES", func() {
				Expect(err).To(MatchError(MatchRegexp("missing CF_INSTANCE_CERT and/or CF_INSTANCE_KEY")))
				Expect(os.Getenv("VCAP_SERVICES")).To(Equal(vcapServicesValue))
			})
		})

		Context("when the system certs path isn't set", func() {
			BeforeEach(func() {
				os.Unsetenv("CF_SYSTEM_CERT_PATH")
			})

			It("prints an error message", func() {
				Expect(err).To(MatchError(MatchRegexp("missing CF_SYSTEM_CERT_PATH")))
				Expect(os.Getenv("VCAP_SERVICES")).To(Equal(vcapServicesValue))
			})
		})

		Context("when credhub skip interpolation is set", func() {
			var originalVCAPServices string

			BeforeEach(func() {
				originalVCAPServices = os.Getenv("VCAP_SERVICES")
				GinkgoT().Setenv("CREDHUB_SKIP_INTERPOLATION", "true")
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/api/v1/interpolate"),
						ghttp.VerifyBody([]byte(vcapServicesValue)),
						VerifyClientCerts(),
						ghttp.RespondWith(http.StatusOK, "JSON_RESPONSE"),
					))
			})

			It("does not change VCAP_SERVICES", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(os.Getenv("VCAP_SERVICES")).To(Equal(originalVCAPServices))
			})
		})

		Context("when VCAP_PLATFORM_OPTIONS is an empty string", func() {
			var originalVCAPServices string
			BeforeEach(func() {
				originalVCAPServices = os.Getenv("VCAP_SERVICES")
				GinkgoT().Setenv("VCAP_PLATFORM_OPTIONS", "")
			})

			It("does not change vcap services", func() {
				Expect(os.Getenv("VCAP_SERVICES")).To(Equal(originalVCAPServices))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when VCAP_PLATFORM_OPTIONS is an empty JSON object", func() {
			var originalVCAPServices string
			BeforeEach(func() {
				GinkgoT().Setenv("VCAP_PLATFORM_OPTIONS", "{}")
				originalVCAPServices = os.Getenv("VCAP_SERVICES")
			})

			It("does not change vcap services", func() {
				Expect(os.Getenv("VCAP_SERVICES")).To(Equal(originalVCAPServices))
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when VCAP_PLATFORM_OPTIONS is an invalid JSON object", func() {
			BeforeEach(func() {
				GinkgoT().Setenv("VCAP_PLATFORM_OPTIONS", `{"credhub-uri":"missing quote and brace`)
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("unable to get platform options: unexpected end of JSON input"))
			})
		})
	})
})
