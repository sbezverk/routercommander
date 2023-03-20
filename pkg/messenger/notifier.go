package messenger

type Notifier interface {
	Notify(string, []byte) error
}
