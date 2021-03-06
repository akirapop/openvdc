// +build linux

package main

import (
	"flag"
	"math/rand"
	"time"
	"strings"
	"log"

	exec "github.com/mesos/mesos-go/executor"
	mesos "github.com/mesos/mesos-go/mesosproto"
	"github.com/samuel/go-zookeeper/zk"
	lxc "gopkg.in/lxc/go-lxc.v2"
	vdc_utils "github.com/axsh/openvdc/util"
)

var (
	slowTasks  = flag.Bool("slow_tasks", false, "")
	lxcpath    string
	template   string
	distro     string
	release    string
	arch       string
	name       string
	verbose    bool
	flush      bool
	validation bool
)

type VDCExecutor struct {
	tasksLaunched int
}

func newVDCExecutor() *VDCExecutor {
	return &VDCExecutor{tasksLaunched: 0}
}

func (exec *VDCExecutor) Registered(driver exec.ExecutorDriver, execInfo *mesos.ExecutorInfo, fwinfo *mesos.FrameworkInfo, slaveInfo *mesos.SlaveInfo) {
	log.Println("Registered Executor on slave ", slaveInfo.GetHostname())
}

func (exec *VDCExecutor) Reregistered(driver exec.ExecutorDriver, slaveInfo *mesos.SlaveInfo) {
	log.Println("Re-registered Executor on slave ", slaveInfo.GetHostname())
}

func (exec *VDCExecutor) Disconnected(driver exec.ExecutorDriver) {
	log.Println("Executor disconnected.")
}

func zkConnect(ip string) *zk.Conn {
	c, _, err := zk.Connect([]string{ip}, time.Second)

	if err != nil {
		log.Println("ERROR: failed connecting to Zookeeper: ", err)
	}

	return c
}

func zkGetData(c *zk.Conn, dir string) []byte {
	data, stat, err := c.Get(dir)

	if err != nil {
		log.Println("ERROR: failed getting data from Zookeeper: ", err)
	}

	log.Println(stat)

	return data[:]
}

func zkSendData(c *zk.Conn, dir string, data string) {
	flags := int32(0)
	acl := zk.WorldACL(zk.PermAll)

	path, err := c.Create(dir, []byte(data), flags, acl)

	if err != nil {
		log.Println("ERROR: failed sending data to Zookeeper: ", err)
	}

	log.Println("Sent: ", data, "to ", dir)
	log.Println(path)
}

func testZkConnection(ip string, dir string, msg string) {

	c := zkConnect(ip)
	zkSendData(c, dir, msg)
	data := []byte(zkGetData(c, dir))
	log.Println(data)
}

func newLxcContainer() {

	c, err := lxc.NewContainer(name, lxcpath)
	if err != nil {
		log.Println("ERROR: %s\n", err)
	}

	log.Println("Creating lxc-container...\n")
	if verbose {
		c.SetVerbosity(lxc.Verbose)
	}

	options := lxc.TemplateOptions{
		Template:             template,
		Distro:               distro,
		Release:              release,
		Arch:                 arch,
		FlushCache:           flush,
		DisableGPGValidation: validation,
	}

	if err := c.Create(options); err != nil {
		log.Println("ERROR: %s\n", err)
	}
}

func destroyLxcContainer() {

	c, err := lxc.NewContainer(name, lxcpath)
        if err != nil {
                log.Println("ERROR: %s\n", err)
        }

	log.Println("Destroying lxc-container..\n")
	if err := c.Destroy(); err != nil {
		log.Println("ERROR: %s\n", err)
	}
}

func startLxcContainer() {

	c, err := lxc.NewContainer(name, lxcpath)
        if err != nil {
                log.Println("ERROR: %s\n", err)
        }

	log.Println("Starting lxc-container...\n")
	if err := c.Start(); err != nil {
		log.Println("ERROR: %s\n", err)
	}

	log.Println("Waiting for lxc-container to start networking\n")
	if _, err := c.WaitIPAddresses(5 * time.Second); err != nil {
		log.Println("ERROR: %s\n", err.Error())
	}
}

func stopLxcContainer() {

	c, err := lxc.NewContainer(name, lxcpath)
        if err != nil {
                log.Println("ERROR: %s\n", err.Error())
        }

	log.Println("Stopping lxc-container..\n")
	if err := c.Stop(); err != nil {
		log.Println("ERROR: %s\n", err.Error())
	}
}

func trimName(untrimmedName string) string {
        limit := "_"
        trimmedName := strings.Split(untrimmedName, limit)[0]

        return trimmedName
}

func newTask(taskName string) {

	trimmedTaskName := trimName(taskName)

        switch trimmedTaskName {
                case "lxc-create":
			log.Println("---Launching task: lxc-create---")
                        newLxcContainer()
                case "lxc-start":
			log.Println("---Launching task: lxc-start---")
                        startLxcContainer()
                case "lxc-stop":
			log.Println("---Launching task: lxc-stop---")
                        stopLxcContainer()
                case "lxc-destroy":
			log.Println("---Launching task: lxc-destroy---")
                        destroyLxcContainer()
                default:
                        log.Println("ERROR: Taskname unrecognized")
        }
}


func (exec *VDCExecutor) LaunchTask(driver exec.ExecutorDriver, taskInfo *mesos.TaskInfo) {
	log.Println("Launching task", taskInfo.GetName(), "with command", taskInfo.Command.GetValue())

	runStatus := &mesos.TaskStatus{
		TaskId: taskInfo.GetTaskId(),
		State:  mesos.TaskState_TASK_RUNNING.Enum(),
	}
	_, err := driver.SendStatusUpdate(runStatus)
	if err != nil {
		log.Println("ERROR: Couldn't send status update", err)
	}

	exec.tasksLaunched++
	log.Println("Tasks launched ", exec.tasksLaunched)


	newTask(taskInfo.GetName())


	finishTask := func() {
		log.Println("Finishing task", taskInfo.GetName())
		finStatus := &mesos.TaskStatus{
			TaskId: taskInfo.GetTaskId(),
			State:  mesos.TaskState_TASK_FINISHED.Enum(),
		}
		if _, err := driver.SendStatusUpdate(finStatus); err != nil {
			log.Println("ERROR: Couldn't send status update", err)
		}
		log.Println("Task finished", taskInfo.GetName())
	}
	if *slowTasks {
		starting := &mesos.TaskStatus{
			TaskId: taskInfo.GetTaskId(),
			State:  mesos.TaskState_TASK_STARTING.Enum(),
		}
		if _, err := driver.SendStatusUpdate(starting); err != nil {
			log.Println("ERROR: Couldn't send status update", err)
		}
		delay := time.Duration(rand.Intn(90)+10) * time.Second
		go func() {
			time.Sleep(delay)
			finishTask()
		}()
	} else {
		finishTask()
	}
}

func (exec *VDCExecutor) KillTask(driver exec.ExecutorDriver, taskID *mesos.TaskID) {
	log.Println("Kill task")
}

func (exec *VDCExecutor) FrameworkMessage(driver exec.ExecutorDriver, msg string) {
	log.Println("Got framework message: ", msg)
}

func (exec *VDCExecutor) Shutdown(driver exec.ExecutorDriver) {
	log.Println("Shutting down the executor")
}

func (exec *VDCExecutor) Error(driver exec.ExecutorDriver, err string) {
	log.Println("Got error message:", err)
}

func init() {
	flag.StringVar(&lxcpath, "lxcpath", lxc.DefaultConfigPath(), "Use specified container path")
	flag.StringVar(&template, "template", "download", "Template to use")
	flag.StringVar(&distro, "distro", "ubuntu", "Template to use")
	flag.StringVar(&release, "release", "trusty", "Template to use")
	flag.StringVar(&arch, "arch", "amd64", "Template to use")
	flag.StringVar(&name, "name", "test", "Name of the container")
	flag.BoolVar(&verbose, "verbose", false, "Verbose output")
	flag.BoolVar(&flush, "flush", false, "Flush the cache")
	flag.BoolVar(&validation, "validation", false, "GPG validation")
	flag.Parse()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {

	vdc_utils.SetupLog("/var/log/axsh/", "vdc-executor.log", "VDC-EXECUTOR: ")

	log.Println("Initializing executor")

	dconfig := exec.DriverConfig{
		Executor: newVDCExecutor(),
	}
	driver, err := exec.NewMesosExecutorDriver(dconfig)

	if err != nil {
		log.Println("ERROR: Couldn't create ExecutorDriver ", err.Error())
	}

	_, err = driver.Start()
	if err != nil {
		log.Println("ERROR: ExecutorDriver wasn't able to start: ", err)
		return
	}
	log.Println("Process running")

	_, err = driver.Join()
	if err != nil {
		log.Println("ERROR: Something went wrong with the driver: ", err)
	}
	log.Println("Executor shutting down")
}
