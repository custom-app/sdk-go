package workerpoolhttp

type Worker struct {
	queue chan *Job
}

func NewWorker(queue chan *Job) *Worker {
	return &Worker{
		queue: queue,
	}
}

// Run - функция, запускающая вечный цикл, который будет слушать очередь до ее закрытия
func (w *Worker) Run() {
	for j := range w.queue {
		j.Handler.ServeHTTP(j.W, j.R)
		j.resCh <- true
	}
}
