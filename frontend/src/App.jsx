import ChatBox from './ChatBox'
import EventLog from './EventLog'
import { useState, useEffect, useCallback } from 'react'
import './App.css'

const getInitialTheme = () => {
  if (typeof window === 'undefined') return 'dark'
  const saved = window.localStorage.getItem('theme')
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

export default function App() {
  const [events, setEvents] = useState([])
  const [theme, setTheme] = useState(getInitialTheme)

  useEffect(() => {
    const html = document.documentElement
    html.classList.toggle('theme-light', theme === 'light')
    html.classList.toggle('theme-dark', theme === 'dark')
    window.localStorage.setItem('theme', theme)
  }, [theme])

  const addEvent = useCallback((message, type = 'info') => {
    setEvents(prev => {
      const next = [...prev, { message, type, id: Date.now() + Math.random() }]
      return next.length > 50 ? next.slice(next.length - 50) : next
    })
  }, [])

  const toggleTheme = useCallback(() => {
    setTheme(prev => (prev === 'dark' ? 'light' : 'dark'))
  }, [])

  return (
    <div className="app">
      <header className="app-header">
        <div className="header-dot" />
        <h1>websocket<span>chat</span></h1>
        <p className="header-sub">real-time · go backend · websocket</p>
        <button className="theme-toggle" onClick={toggleTheme} aria-label="Toggle dark and light mode">
          {theme === 'dark' ? 'Light mode' : 'Dark mode'}
        </button>
      </header>

      <main className="app-main">
        <ChatBox user="Alex" addEvent={addEvent} />
        <ChatBox user="Bob" addEvent={addEvent} />
        <EventLog events={events} />
      </main>
    </div>
  )
}
