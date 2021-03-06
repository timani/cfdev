package cmd_test

import (
	"encoding/binary"
	"net"

	. "code.cloudfoundry.org/cfdev/cmd"

	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"code.cloudfoundry.org/cfdev/config"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Stop", func() {
	var linuxkit, vpnkit, hyperkit *gexec.Session
	var stop Stop
	var state, linuxkitPidPath, hyperkitPidPath, vpnkitPidPath string

	BeforeEach(func() {
		state, _ = ioutil.TempDir("", "pcfdev.stop.")

		linuxkitPidPath = filepath.Join(state, "linuxkit.pid")
		hyperkitPidPath = filepath.Join(state, "hyperkit.pid")
		vpnkitPidPath = filepath.Join(state, "vpnkit.pid")

		stop = Stop{
			Config: config.Config{
				LinuxkitPidFile: linuxkitPidPath,
				HyperkitPidFile: hyperkitPidPath,
				VpnkitPidFile:   vpnkitPidPath,
			},
		}
	})
	Context("all processes are running and pid files exist", func() {
		BeforeEach(func() {
			linuxkit, _ = gexec.Start(exec.Command("sleep", "100"), GinkgoWriter, GinkgoWriter)
			vpnkit, _ = gexec.Start(exec.Command("sleep", "100"), GinkgoWriter, GinkgoWriter)
			hyperkit, _ = gexec.Start(exec.Command("sleep", "100"), GinkgoWriter, GinkgoWriter)

			ioutil.WriteFile(linuxkitPidPath, []byte(strconv.Itoa(linuxkit.Command.Process.Pid)), 0644)
			ioutil.WriteFile(vpnkitPidPath, []byte(strconv.Itoa(vpnkit.Command.Process.Pid)), 0644)
			ioutil.WriteFile(hyperkitPidPath, []byte(strconv.Itoa(hyperkit.Command.Process.Pid)), 0644)
		})

		AfterEach(func() {
			os.RemoveAll(state)

			linuxkit.Terminate()
			vpnkit.Terminate()
			hyperkit.Terminate()
		})

		It("kill all Pids", func() {
			Expect(stop.Run([]string{})).To(Succeed())

			Eventually(linuxkit).Should(gexec.Exit())
			Eventually(vpnkit).Should(gexec.Exit())
			Eventually(hyperkit).Should(gexec.Exit())
		})

		It("removes the pid files", func() {
			Expect(stop.Run([]string{})).To(Succeed())

			Expect(linuxkitPidPath).ToNot(BeAnExistingFile())
			Expect(vpnkitPidPath).ToNot(BeAnExistingFile())
			Expect(hyperkitPidPath).ToNot(BeAnExistingFile())
		})
	})

	Context("all pidfiles are missing", func() {
		It("does nothing and succeeds", func() {
			Expect(stop.Run([]string{})).To(Succeed())
		})
	})

	Context("one pid file is missing", func() {
		BeforeEach(func() {
			os.Remove(vpnkitPidPath)
		})

		It("kills existing pids", func() {
			Expect(stop.Run([]string{})).To(Succeed())

			Expect(linuxkitPidPath).ToNot(BeAnExistingFile())
			Expect(hyperkitPidPath).ToNot(BeAnExistingFile())
		})
	})

	Context("one process has stopped, pid file exists", func() {
		BeforeEach(func() {
			linuxkit, _ = gexec.Start(exec.Command("sleep", "100"), GinkgoWriter, GinkgoWriter)
			vpnkit, _ = gexec.Start(exec.Command("sleep", "100"), GinkgoWriter, GinkgoWriter)
			hyperkit, _ = gexec.Start(exec.Command("sleep", "100"), GinkgoWriter, GinkgoWriter)

			vpnkit.Kill()

			ioutil.WriteFile(linuxkitPidPath, []byte(strconv.Itoa(linuxkit.Command.Process.Pid)), 0644)
			ioutil.WriteFile(vpnkitPidPath, []byte(strconv.Itoa(vpnkit.Command.Process.Pid)), 0644)
			ioutil.WriteFile(hyperkitPidPath, []byte(strconv.Itoa(hyperkit.Command.Process.Pid)), 0644)
		})

		AfterEach(func() {
			os.RemoveAll(state)
		})

		It("kills existing pids and returns error", func() {
			Expect(stop.Run([]string{})).To(Succeed())

			Expect(linuxkit).To(gexec.Exit())
			Expect(hyperkit).To(gexec.Exit())

			Expect(linuxkitPidPath).ToNot(BeAnExistingFile())
			Expect(vpnkitPidPath).ToNot(BeAnExistingFile())
			Expect(hyperkitPidPath).ToNot(BeAnExistingFile())
		})
	})

	Context("all processes have exited and all pidfiles exists", func() {
		BeforeEach(func() {
			proc, _ := gexec.Start(exec.Command("echo", "100"), GinkgoWriter, GinkgoWriter)
			Eventually(proc).Should(gexec.Exit(0))
			pid := []byte(strconv.Itoa(proc.Command.Process.Pid))

			ioutil.WriteFile(linuxkitPidPath, pid, 0644)
			ioutil.WriteFile(vpnkitPidPath, pid, 0644)
			ioutil.WriteFile(hyperkitPidPath, pid, 0644)
		})

		AfterEach(func() {
			os.RemoveAll(state)
		})

		It("deletes all pid files and succeeds", func() {
			Expect(stop.Run([]string{})).To(Succeed())

			Expect(linuxkitPidPath).ToNot(BeAnExistingFile())
			Expect(vpnkitPidPath).ToNot(BeAnExistingFile())
			Expect(hyperkitPidPath).ToNot(BeAnExistingFile())
		})
	})

	Context("cfdevd socket exists", func() {
		var tmpDir string
		var instructions chan byte
		var uninstallErrorCode int
		BeforeEach(func() {
			tmpDir, _ = ioutil.TempDir("", "cfdev.stop.")
			stop.Config.CFDevDSocketPath = filepath.Join(tmpDir, "cfdevd.socket")
			instructions = make(chan byte, 1)
			ln, err := net.Listen("unix", stop.Config.CFDevDSocketPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(stop.Config.CFDevDSocketPath).To(BeAnExistingFile())
			go func() {
				conn, err := ln.Accept()
				Expect(err).NotTo(HaveOccurred())
				handshake := make([]byte, 49, 49)
				binary.Read(conn, binary.LittleEndian, handshake)
				binary.Write(conn, binary.LittleEndian, handshake)
				instruction := make([]byte, 1, 1)
				binary.Read(conn, binary.LittleEndian, instruction)
				instructions <- instruction[0]
				if uninstallErrorCode == -1 {
					conn.Close()
				} else {
					binary.Write(conn, binary.LittleEndian, []byte{byte(uninstallErrorCode)})
				}
			}()
		})
		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})
		It("succeeds and sends the uninstall command to cfdevd", func() {
			uninstallErrorCode = 0
			Expect(stop.Run([]string{})).To(Succeed())

			Eventually(instructions).Should(Receive(Equal(byte(1))))
		})
		Context("cfdevd stops after receiving uninstall command, thus closes connection before writing success code", func() {
			It("succeeds", func() {
				uninstallErrorCode = -1
				Expect(stop.Run([]string{})).To(Succeed())

				Eventually(instructions).Should(Receive(Equal(byte(1))))
			})
		})
		Context("cfdevd returns error to uninstall", func() {
			It("returns the error", func() {
				uninstallErrorCode = 1
				Expect(stop.Run([]string{})).To(MatchError("failed to uninstall cfdevd: errorcode: 1"))
			})
		})
	})
	Context("cfdevd socket is specified but does not exist", func() {
		var tmpDir string
		BeforeEach(func() {
			tmpDir, _ = ioutil.TempDir("", "cfdev.stop.")
			stop.Config.CFDevDSocketPath = filepath.Join(tmpDir, "cfdevd.socket")
		})
		AfterEach(func() {
			os.RemoveAll(tmpDir)
		})
		It("succeeds", func() {
			Expect(stop.Run([]string{})).To(Succeed())
		})
	})
})
