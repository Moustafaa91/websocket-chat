import { useEffect, useRef } from 'react'

const TYPE_LABEL = {
  success: 'OK',
  warn: '!',
  error: 'X',
  info: 'i',
}

export default function EventLog({ events }) {
  const bottomRef = useRef(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [events])

  function formatTime(ts) {
    return new Date(ts).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  }

  return (
    <div className="eventlog">
      <div className="eventlog-header">
        <span>event log</span>
        <span className="eventlog-count">{events.length} / 50</span>
      </div>
      <div className="eventlog-body">
        {events.length === 0 && (
          <p className="eventlog-empty">waiting for events...</p>
        )}
        {events.map(ev => (
          <div key={ev.id} className={`event event--${ev.type}`}>
            <span className="event-icon">{TYPE_LABEL[ev.type] ?? 'i'}</span>
            <span className="event-time">{formatTime(ev.time)}</span>
            <span className="event-msg">{ev.message}</span>
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}
