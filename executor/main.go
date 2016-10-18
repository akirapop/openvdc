package main

import (
        "flag"
        "fmt"
        "math/rand"
        "time"
        exec "github.com/mesos/mesos-go/executor"
        mesos "github.com/mesos/mesos-go/mesosproto"
        "github.com/samuel/go-zookeeper/zk"
)

var (
        slowTasks = flag.Bool("slow_tasks", false, "When true tasks will take several seconds before responding with TASK_FINISHED; useful for debugging failover")
)

type VDCExecutor struct {
        tasksLaunched int
}

func newVDCExecutor() *VDCExecutor {
        return &VDCExecutor{tasksLaunched: 0}
}

func (exec *VDCExecutor) Registered(driver exec.ExecutorDriver, execInfo *mesos.ExecutorInfo, fwinfo *mesos.FrameworkInfo, slaveInfo *mesos.SlaveInfo) {
        fmt.Println("Registered Executor on slave ", slaveInfo.GetHostname())
}

func (exec *VDCExecutor) Reregistered(driver exec.ExecutorDriver, slaveInfo *mesos.SlaveInfo) {
        fmt.Println("Re-registered Executor on slave ", slaveInfo.GetHostname())
}

func (exec *VDCExecutor) Disconnected(driver exec.ExecutorDriver) {
        fmt.Println("Executor disconnected.")
}

func (exec *VDCExecutor) LaunchTask(driver exec.ExecutorDriver, taskInfo *mesos.TaskInfo) {
        fmt.Println("Launching task", taskInfo.GetName(), "with command", taskInfo.Command.GetValue())

        runStatus := &mesos.TaskStatus{
                TaskId: taskInfo.GetTaskId(),
                State:  mesos.TaskState_TASK_RUNNING.Enum(),
        }
        _, err := driver.SendStatusUpdate(runStatus)
        if err != nil {
                fmt.Println("Got error", err)
        }

        exec.tasksLaunched++
        fmt.Println("Total tasks launched ", exec.tasksLaunched)


         //Zookeeper connection test
        c, _, err := zk.Connect([]string{"127.0.0.1"}, time.Second) 

	defer c.Close()

        flags := int32(0)
        acl := zk.WorldACL(zk.PermAll)

        path, err := c.Create("/01", []byte("data"), flags, acl)
        must(err)
        fmt.Printf("create: %+v\n", path)

        data, stat, err := c.Get("/01")
        must(err)
        fmt.Printf("get:    %+v %+v\n", string(data), stat)

        stat, err = c.Set("/01", []byte("newdata"), stat.Version)
        must(err)
        fmt.Printf("set:    %+v\n", stat)

        err = c.Delete("/01", -1)
        must(err)
        fmt.Printf("delete: ok\n")

        exists, stat, err := c.Exists("/01")
        must(err)
        fmt.Printf("exists: %+v %+v\n", exists, stat)


	finishTask := func() {
                fmt.Println("Finishing task", taskInfo.GetName())
                finStatus := &mesos.TaskStatus{
                        TaskId: taskInfo.GetTaskId(),
                        State:  mesos.TaskState_TASK_FINISHED.Enum(),
                }
                if _, err := driver.SendStatusUpdate(finStatus); err != nil {
                        fmt.Println("error sending FINISHED", err)
                }
                fmt.Println("Task finished", taskInfo.GetName())
        }
        if *slowTasks {
                starting := &mesos.TaskStatus{
                        TaskId: taskInfo.GetTaskId(),
                        State:  mesos.TaskState_TASK_STARTING.Enum(),
                }
                if _, err := driver.SendStatusUpdate(starting); err != nil {
                        fmt.Println("error sending STARTING", err)
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
        fmt.Println("Kill task")
}

func (exec *VDCExecutor) FrameworkMessage(driver exec.ExecutorDriver, msg string) {
        fmt.Println("Got framework message: ", msg)
}

func (exec *VDCExecutor) Shutdown(driver exec.ExecutorDriver) {
        fmt.Println("Shutting down the executor")
}

func (exec *VDCExecutor) Error(driver exec.ExecutorDriver, err string) {
        fmt.Println("Got error message:", err)
}

func init() {

}

func must(err error) {
        if err != nil {
                panic(err)
        }
}

func main() {
        fmt.Println("Starting VDC Executor (Go)")

        dconfig := exec.DriverConfig{
                Executor: newVDCExecutor(),
        }
        driver, err := exec.NewMesosExecutorDriver(dconfig)

        if err != nil {
                fmt.Println("Unable to create a ExecutorDriver ", err.Error())
        }

        _, err = driver.Start()
        if err != nil {
                fmt.Println("Got error:", err)
                return
        }
        fmt.Println("Executor process has started and running.")

	_, err = driver.Join()
        if err != nil {
                fmt.Println("driver failed:", err)
        }
        fmt.Println("executor terminating")
}

