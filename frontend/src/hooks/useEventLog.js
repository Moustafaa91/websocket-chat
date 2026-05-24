import { useState, useCallback, useRef } from 'react'

const MAX_EVENTS = 50

export function useEventLog() {
  const [events, setEvents] = useState([])
  const counterRef = useRef(0)

  const addEvent = useCallback((message, type = 'info') => {
    const id = counterRef.current++
    setEvents(prev => {
      const next = [...prev, { message, type, time: Date.now(), id }]
      return next.length > MAX_EVENTS ? next.slice(-MAX_EVENTS) : next
    })
  }, [])

  return { events, addEvent }
}
