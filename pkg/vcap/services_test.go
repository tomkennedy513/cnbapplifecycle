package vcap_test

import (
	"os"
	"path/filepath"
	"testing"

	"code.cloudfoundry.org/cnbapplifecycle/pkg/vcap"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVCAP(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VCAP_SERVICES Suite")
}

var _ = Describe("vcap", func() {
	Describe("TranslateVcapServices", func() {
		var (
			vcapServicesValue string
			bindingRoot       string
			err               error
		)

		var _ = BeforeEach(func() {
			var beforeErr error
			bindingRoot, beforeErr = os.MkdirTemp("", "bindings")
			Expect(beforeErr).NotTo(HaveOccurred())
		})

		var _ = AfterEach(func() {
			err := os.RemoveAll(bindingRoot)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			err = vcap.TranslateVcapServices(bindingRoot)
		})

		Context("when translating valid vcap services", func() {
			BeforeEach(func() {
				vcapServicesValue = `
				{
				"user-provided": [{"name": "binding1", "label": "user-provided", "credentials":{"type":"ca-certificates", "cert.pem": "my-cert"}}],
				"postgres":[{"name": "binding2", "label":"postgres", "credentials":{"uri":"postgresql://exampleuser:examplepass@mydb.com:5432/exampleuser"}}]
				}`
				GinkgoT().Setenv("VCAP_SERVICES", vcapServicesValue)
			})

			It("does not fail and sets SERVICE_BINDING_ROOT", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(os.Getenv("SERVICE_BINDING_ROOT")).To(Equal(bindingRoot))
			})

			It("writes credentials as kubernetes type service bindings", func() {
				Expect(filepath.Join(bindingRoot, "binding1", "type")).To(BeAnExistingFile())
				contents, err := os.ReadFile(filepath.Join(bindingRoot, "binding1", "type"))
				Expect(err).NotTo(HaveOccurred())
				Expect(contents).To(Equal([]byte("ca-certificates")))

				Expect(filepath.Join(bindingRoot, "binding1", "cert.pem")).To(BeAnExistingFile())
				contents, err = os.ReadFile(filepath.Join(bindingRoot, "binding1", "cert.pem"))
				Expect(err).NotTo(HaveOccurred())
				Expect(contents).To(Equal([]byte("my-cert")))

				Expect(filepath.Join(bindingRoot, "binding2", "type")).ToNot(BeAnExistingFile())
			

				Expect(filepath.Join(bindingRoot, "binding2", "uri")).To(BeAnExistingFile())
				contents, err = os.ReadFile(filepath.Join(bindingRoot, "binding2", "uri"))
				Expect(err).NotTo(HaveOccurred())
				Expect(contents).To(Equal([]byte("postgresql://exampleuser:examplepass@mydb.com:5432/exampleuser")))

			})

			It("does not create a type file if it does not exist", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(filepath.Join(bindingRoot, "binding2", "type")).ToNot(BeAnExistingFile())
			})

			It("sets DATABASE_URL when valid uris are present", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(os.Getenv("DATABASE_URL")).To(Equal("postgres://exampleuser:examplepass@mydb.com:5432/exampleuser"))
			})
		})
	})
})
