package databaseuri_test

import (
	"code.cloudfoundry.org/cnbapplifecycle/pkg/databaseuri"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDatabaseuri(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Databaseuri Suite")
}

var _ = Describe("GetDatabaseUriFromVcapServices", func() {

	Context("with valid database schemes", func() {
		It("converts mysql scheme to mysql2", func() {
			services := []byte(`{"mysql-service":[{"credentials":{"uri":"mysql://username:password@example.com/db"}}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal("mysql2://username:password@example.com/db"))
		})

		It("keeps mysql2 scheme unchanged", func() {
			services := []byte(`{"mysql-service":[{"credentials":{"uri":"mysql2://username:password@example.com/db"}}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal("mysql2://username:password@example.com/db"))
		})

		It("keeps postgres scheme unchanged", func() {
			services := []byte(`{"postgres-service":[{"credentials":{"uri":"postgres://username:password@example.com/db"}}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal("postgres://username:password@example.com/db"))
		})

		It("converts postgresql scheme to postgres", func() {
			services := []byte(`{"postgres-service":[{"credentials":{"uri":"postgresql://username:password@example.com/db"}}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal("postgres://username:password@example.com/db"))
		})
	})

	Context("with multiple services", func() {
		It("returns a valid database URI when multiple services exist", func() {
			services := []byte(`{
				"sendgrid":[{"credentials":{"uri":"sendgrid://user:pass@example.com/service"}}],
				"postgres-service":[{"credentials":{"uri":"postgres://username:password@example.com/db1"}}],
				"mysql-service":[{"credentials":{"uri":"mysql://username:password@example.com/db2"}}]
			}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(SatisfyAny(
				Equal("postgres://username:password@example.com/db1"),
				Equal("mysql2://username:password@example.com/db2"),
			))
		})

		It("skips services without uri and returns a valid database URI", func() {
			services := []byte(`{
				"non-db-service":[{"credentials":{"other":"data"}}],
				"empty-service":[{}],
				"mysql-service":[{"credentials":{"uri":"mysql://username:password@example.com/db"}}]
			}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal("mysql2://username:password@example.com/db"))
		})

		It("skips invalid URIs and returns a valid one", func() {
			services := []byte(`{
				"invalid-service":[{"credentials":{"uri":"postgresql://invalid:some-pass@example.com/%a"}}],
				"valid-service":[{"credentials":{"uri":"postgres://username:password@example.com/db"}}]
			}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal("postgres://username:password@example.com/db"))
		})

		It("handles multiple bindings within the same service", func() {
			services := []byte(`{
				"postgres-service":[
					{"credentials":{"other":"data"}},
					{"credentials":{"uri":"postgres://some-user:some-pass@example.com/db1"}},
					{"credentials":{"uri":"postgres://username:password@example.com/db2"}}
				]
			}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(SatisfyAny(
				Equal("postgres://some-user:some-pass@example.com/db1"),
				Equal("postgres://username:password@example.com/db2"),
			))
		})
	})

	Context("with unsupported database schemes", func() {
		It("returns empty string for unsupported database schemes", func() {
			services := []byte(`{"redis-service":[{"credentials":{"uri":"redis://username:password@example.com/db"}}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal(""))
		})

		It("returns empty string for non-database services", func() {
			services := []byte(`{"sendgrid":[{"credentials":{"uri":"sendgrid://foo:bars@example.com/service"}}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal(""))
		})

		It("returns empty string for HTTP services", func() {
			services := []byte(`{"web-service":[{"credentials":{"uri":"http://api.example.com/my-service"}}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal(""))
		})
	})

	Context("with invalid or missing data", func() {
		It("returns empty string when services have no credentials", func() {
			services := []byte(`{"service":[{}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal(""))
		})

		It("returns empty string when credentials exist but no uri field", func() {
			services := []byte(`{"service":[{"credentials":{"username":"some-user","password":"some-pass"}}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal(""))
		})

		It("returns empty string when credentials.uri is empty", func() {
			services := []byte(`{"service":[{"credentials":{"uri":""}}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal(""))
		})

		It("returns empty string when no services exist", func() {
			services := []byte(`{}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal(""))
		})

		It("returns empty string when services array is empty", func() {
			services := []byte(`{"service":[]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal(""))
		})

		It("handles malformed URIs gracefully", func() {
			services := []byte(`{"service":[{"credentials":{"uri":"/not|-a-valid-uri"}}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal(""))
		})

		It("handles URIs with special characters that cause parsing errors", func() {
			services := []byte(`{"service":[{"credentials":{"uri":"postgresql://invalid:some-pass@example.com/%a%b%c"}}]}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal(""))
		})

		It("returns empty string for empty input", func() {
			services := []byte(``)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal(""))
		})
	})

	Context("with invalid JSON", func() {
		It("returns an error for completely malformed JSON", func() {
			services := []byte(`{"service":[{"credentials":{"uri":"postgres://some-user:some-pass@example.com/db"}`)
			_, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error for invalid JSON structure", func() {
			services := []byte(`invalid json`)
			_, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("with complex realistic VCAP_SERVICES", func() {
		It("extracts database URI from complex multi-service VCAP_SERVICES", func() {
			services := []byte(`{
				"sendgrid": [
					{
						"credentials": {
							"hostname": "something.example.com",
							"username": "apikey",
							"password": "some-pass"
						},
						"instance_name": "my-sendgrid",
						"label": "sendgrid",
						"name": "my-sendgrid"
					}
				],
				"postgres": [
					{
						"credentials": {
							"uri": "postgres://some-user:some-pass@db.example.com:5432/dbname?sslmode=require",
							"hostname": "db.example.com",
							"port": 5432,
							"username": "username",
							"password": "some-pass",
							"database": "dbname"
						},
						"instance_name": "my-postgres",
						"label": "postgres",
						"name": "my-postgres"
					}
				],
				"redis": [
					{
						"credentials": {
							"uri": "redis://some-user:some-pass@redis.example.com:6379/database",
							"hostname": "redis.example.com",
							"port": 6379,
							"password": "some-pass"
						},
						"instance_name": "my-redis",
						"label": "redis",
						"name": "my-redis"
					}
				]
			}`)
			uri, err := databaseuri.GetDatabaseUriFromVcapServices(services)
			Expect(err).NotTo(HaveOccurred())
			Expect(uri).To(Equal("postgres://some-user:some-pass@db.example.com:5432/dbname?sslmode=require"))
		})
	})
})
