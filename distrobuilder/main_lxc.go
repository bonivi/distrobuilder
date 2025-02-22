package main

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/lxc/distrobuilder/generators"
	"github.com/lxc/distrobuilder/image"
	"github.com/lxc/distrobuilder/managers"
	"github.com/lxc/distrobuilder/shared"
)

type cmdLXC struct {
	cmdBuild *cobra.Command
	cmdPack  *cobra.Command
	global   *cmdGlobal
}

func (c *cmdLXC) commandBuild() *cobra.Command {
	c.cmdBuild = &cobra.Command{
		Use:     "build-lxc <filename|-> [target dir]",
		Short:   "Build LXC image from scratch",
		Args:    cobra.RangeArgs(1, 2),
		PreRunE: c.global.preRunBuild,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := c.global.logger

			cleanup, overlayDir, err := getOverlay(logger, c.global.flagCacheDir, c.global.sourceDir)
			if err != nil {
				logger.Warnw("Failed to creaty overlay", "err", err)

				overlayDir = filepath.Join(c.global.flagCacheDir, "overlay")

				// Use rsync if overlay doesn't work
				err = shared.RunCommand("rsync", "-a", c.global.sourceDir+"/", overlayDir)
				if err != nil {
					return errors.Wrap(err, "Failed to copy image content")
				}
			} else {
				defer cleanup()
			}

			return c.run(cmd, args, overlayDir)
		},
	}
	return c.cmdBuild
}

func (c *cmdLXC) commandPack() *cobra.Command {
	c.cmdPack = &cobra.Command{
		Use:     "pack-lxc <filename|-> <source dir> [target dir]",
		Short:   "Create LXC image from existing rootfs",
		Args:    cobra.RangeArgs(2, 3),
		PreRunE: c.global.preRunPack,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := c.global.logger

			cleanup, overlayDir, err := getOverlay(logger, c.global.flagCacheDir, c.global.sourceDir)
			if err != nil {
				logger.Warnw("Failed to creaty overlay", "err", err)

				overlayDir = filepath.Join(c.global.flagCacheDir, "overlay")

				// Use rsync if overlay doesn't work
				err = shared.RunCommand("rsync", "-a", c.global.sourceDir+"/", overlayDir)
				if err != nil {
					return errors.Wrap(err, "Failed to copy image content")
				}
			} else {
				defer cleanup()
			}

			err = c.runPack(cmd, args, overlayDir)
			if err != nil {
				return err
			}

			return c.run(cmd, args, overlayDir)
		},
	}
	return c.cmdPack
}

func (c *cmdLXC) runPack(cmd *cobra.Command, args []string, overlayDir string) error {
	// Setup the mounts and chroot into the rootfs
	exitChroot, err := shared.SetupChroot(overlayDir, c.global.definition.Environment, nil)
	if err != nil {
		return errors.Wrap(err, "Failed to setup chroot")
	}
	// Unmount everything and exit the chroot
	defer exitChroot()

	var manager *managers.Manager
	imageTargets := shared.ImageTargetAll | shared.ImageTargetContainer

	if c.global.definition.Packages.Manager != "" {
		manager = managers.Get(c.global.definition.Packages.Manager)
		if manager == nil {
			return fmt.Errorf("Couldn't get manager")
		}
	} else {
		manager = managers.GetCustom(*c.global.definition.Packages.CustomManager)
	}

	err = manageRepositories(c.global.definition, manager, imageTargets)
	if err != nil {
		return errors.Wrap(err, "Failed to manage repositories")
	}

	// Run post unpack hook
	for _, hook := range c.global.definition.GetRunnableActions("post-unpack", imageTargets) {
		err := shared.RunScript(hook.Action)
		if err != nil {
			return errors.Wrap(err, "Failed to run post-unpack")
		}
	}

	// Install/remove/update packages
	err = managePackages(c.global.definition, manager, imageTargets)
	if err != nil {
		return errors.Wrap(err, "Failed to manage packages")
	}

	// Run post packages hook
	for _, hook := range c.global.definition.GetRunnableActions("post-packages", imageTargets) {
		err := shared.RunScript(hook.Action)
		if err != nil {
			return errors.Wrap(err, "Failed to run post-packages")
		}
	}

	return nil
}

func (c *cmdLXC) run(cmd *cobra.Command, args []string, overlayDir string) error {
	img := image.NewLXCImage(overlayDir, c.global.targetDir,
		c.global.flagCacheDir, *c.global.definition)

	for _, file := range c.global.definition.Files {
		generator := generators.Get(file.Generator)
		if generator == nil {
			return fmt.Errorf("Unknown generator '%s'", file.Generator)
		}

		if !shared.ApplyFilter(&file, c.global.definition.Image.Release, c.global.definition.Image.ArchitectureMapped, c.global.definition.Image.Variant, c.global.definition.Targets.Type, shared.ImageTargetUndefined|shared.ImageTargetAll|shared.ImageTargetContainer) {
			continue
		}

		err := generator.RunLXC(c.global.flagCacheDir, overlayDir, img,
			c.global.definition.Targets.LXC, file)
		if err != nil {
			return err
		}
	}

	exitChroot, err := shared.SetupChroot(overlayDir,
		c.global.definition.Environment, nil)
	if err != nil {
		return err
	}

	fixCapabilities()

	// Run post files hook
	for _, action := range c.global.definition.GetRunnableActions("post-files", shared.ImageTargetAll|shared.ImageTargetContainer) {
		err := shared.RunScript(action.Action)
		if err != nil {
			exitChroot()
			return errors.Wrap(err, "Failed to run post-files")
		}
	}

	exitChroot()

	err = img.Build()
	if err != nil {
		return errors.Wrap(err, "Failed to create LXC image")
	}

	return nil
}
