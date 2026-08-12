package action_test

import (
	"context"

	"github.com/urfave/cli/v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/cli/action"
	cmdpkg "github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
)

var _ = Describe("Customize action", Label("customize"), func() {
	var cleanup func()

	BeforeEach(func() {
		cmdpkg.CustomizeArgs = cmdpkg.CustomizeFlags{
			ConfigDir:  "/config",
			MediaType:  "raw",
			Mode:       "embedded",
			OutputPath: "/out/image.raw",
			Platform:   "linux/amd64",
		}
	})

	AfterEach(func() {
		cmdpkg.CustomizeArgs = cmdpkg.CustomizeFlags{}
		if cleanup != nil {
			cleanup()
		}
	})

	It("rejects dynamic Kubernetes configuration outside merge mode", func() {
		fs, c, err := sysmock.TestFS(map[string]any{
			"/config/install.yaml":         "schema: v0\nbootloader: grub\nraw:\n  diskSize: 35G\n",
			"/config/release.yaml":         "manifestURI: oci://registry.example.com/release:1\n",
			"/config/dynamic_service.yaml": "services:\n  k8s-dynamic:\n    enabled: true\n",
		})
		Expect(err).NotTo(HaveOccurred())
		cleanup = c
		system, err := sys.NewSystem(sys.WithFS(fs), sys.WithLogger(log.New()))
		Expect(err).NotTo(HaveOccurred())
		cmd := &cli.Command{
			Metadata: map[string]any{
				"system": system,
			},
		}

		err = action.Customize(context.Background(), cmd)

		Expect(err).To(MatchError(ContainSubstring("dynamic Kubernetes configuration requires --mode merge")))
	})
})
