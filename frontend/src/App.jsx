import { useState, useCallback, useEffect, useRef } from 'react'
import ChatRoom from './components/ChatRoom'
import EventLog from './components/EventLog'
import HomeScreen from './screens/HomeScreen'
import WaitingScreen from './screens/WaitingScreen'
import JoiningScreen from './screens/JoiningScreen'
import { createRoom, validateRoom, wakeBackend } from './api/rooms'
import { useTheme } from './hooks/useTheme'
import { useEventLog } from './hooks/useEventLog'
import './App.css'

export default function App() {
  const [screen, setScreen] = useState('home')
  const [roomCode, setRoomCode] = useState('')
  const [playerNum, setPlayerNum] = useState(null)
  const [joinInput, setJoinInput] = useState('')
  const [joinError, setJoinError] = useState('')
  const [joinLoading, setJoinLoading] = useState(false)
  const [createLoading, setCreateLoading] = useState(false)
  const [createStatus, setCreateStatus] = useState('')

  const roomWsRef = useRef(null)
  const wsConnectStartedRef = useRef(false)
  const createStatusTimersRef = useRef([])

  const { theme, toggleTheme } = useTheme()
  const { events, addEvent } = useEventLog()

  const clearCreateStatusTimers = useCallback(() => {
    createStatusTimersRef.current.forEach(clearTimeout)
    createStatusTimersRef.current = []
  }, [])

  useEffect(() => {
    wakeBackend().catch(() => {
      // The create/join actions surface connection errors when the user needs them.
    })
  }, [])

  useEffect(() => clearCreateStatusTimers, [clearCreateStatusTimers])

  const resetSession = useCallback(() => {
    wsConnectStartedRef.current = false
    setRoomCode('')
    setPlayerNum(null)
  }, [])

  const handleCreate = useCallback(async () => {
    clearCreateStatusTimers()
    setCreateLoading(true)
    setCreateStatus('Contacting the chat server...')
    createStatusTimersRef.current = [
      setTimeout(() => {
        setCreateStatus('The free Render backend is waking up. This can take about a minute after idle time.')
      }, 2500),
      setTimeout(() => {
        setCreateStatus('Still waking the server. Please keep this tab open; your room will appear automatically.')
      }, 15000),
    ]

    try {
      await wakeBackend()
      const code = await createRoom()
      setRoomCode(code)
      setPlayerNum(1)
      wsConnectStartedRef.current = false
      setScreen('waiting')
      addEvent(`Room ${code} created - share the code with your friend`, 'success')
    } catch (err) {
      addEvent('Could not create room: ' + err.message, 'error')
    } finally {
      clearCreateStatusTimers()
      setCreateStatus('')
      setCreateLoading(false)
    }
  }, [addEvent, clearCreateStatusTimers])

  const handleCancelWaiting = useCallback(() => {
    if (roomWsRef.current) {
      roomWsRef.current.close(1000, 'cancelled')
      roomWsRef.current = null
    }
    resetSession()
    setScreen('home')
    addEvent('Room cancelled', 'warn')
  }, [addEvent, resetSession])

  const handlePlayer1Ready = useCallback((ws, code) => {
    roomWsRef.current = ws
    setScreen('chat')
    addEvent(`Player 2 joined room ${code} - opening chat`, 'success')
  }, [addEvent])

  const handleJoinSubmit = useCallback(async () => {
    const code = joinInput.trim().toUpperCase()
    if (code.length !== 6) {
      setJoinError('Code must be 6 characters')
      return
    }

    setJoinError('')
    setJoinLoading(true)
    let targetPlayer = 2
    try {
      try {
        await validateRoom(code, 2)
      } catch {
        await validateRoom(code, 1)
        targetPlayer = 1
      }
    } catch (err) {
      setJoinError(err.message)
      addEvent(err.message, 'error')
      return
    } finally {
      setJoinLoading(false)
    }

    setRoomCode(code)
    setPlayerNum(targetPlayer)
    wsConnectStartedRef.current = false
    setScreen('joining')
    addEvent(`Joining room ${code} as Player ${targetPlayer}...`, 'info')
  }, [joinInput, addEvent])

  const handleJoinSuccess = useCallback((ws, code) => {
    roomWsRef.current = ws
    setScreen('chat')
    addEvent(`Joined room ${code}`, 'success')
  }, [addEvent])

  const handleJoinFailure = useCallback((message) => {
    if (roomWsRef.current) {
      roomWsRef.current.close(1000, 'join failed')
      roomWsRef.current = null
    }
    resetSession()
    setJoinError(message)
    setScreen('home')
    addEvent(message, 'error')
  }, [addEvent, resetSession])

  const handleChatEnd = useCallback((reason) => {
    roomWsRef.current = null
    resetSession()
    addEvent(reason || 'Chat ended', 'warn')
    setScreen('home')
    setJoinInput('')
  }, [addEvent, resetSession])

  return (
    <div className="app">
      <header className="app-header">
        <div className="header-dot" />
        <h1>websocket<span>chat</span></h1>
        <p className="header-sub">real-time / go backend / websocket</p>
        <a
          className="source-link"
          href="https://github.com/Moustafaa91/websocket-chat"
          target="_blank"
          rel="noreferrer"
        >
          <svg
            aria-hidden="true"
            className="source-link-icon"
            viewBox="0 0 24 24"
            fill="currentColor"
          >
            <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.1 3.29 9.42 7.86 10.95.58.1.79-.25.79-.56v-2.15c-3.2.7-3.88-1.36-3.88-1.36-.52-1.34-1.28-1.7-1.28-1.7-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.76 2.7 1.25 3.36.96.1-.75.4-1.25.73-1.54-2.56-.29-5.25-1.28-5.25-5.69 0-1.26.45-2.29 1.19-3.09-.12-.29-.52-1.47.11-3.05 0 0 .98-.31 3.18 1.18A11.02 11.02 0 0 1 12 6.05c.98 0 1.95.13 2.86.39 2.2-1.49 3.17-1.18 3.17-1.18.64 1.58.24 2.76.12 3.05.74.8 1.18 1.83 1.18 3.09 0 4.42-2.7 5.39-5.26 5.68.42.36.79 1.07.79 2.16v3.15c0 .31.21.67.8.56A11.51 11.51 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5Z" />
          </svg>
          Moustafaa91/websocket-chat
        </a>
        <button type="button" className="theme-toggle" onClick={toggleTheme}>
          {theme === 'dark' ? 'Light mode' : 'Dark mode'}
        </button>
      </header>

      <main className="app-main app-main--split">
        <div className="app-content">
          {screen === 'home' && (
            <HomeScreen
              joinInput={joinInput}
              joinError={joinError}
              joinLoading={joinLoading}
              onJoinInputChange={e => {
                setJoinInput(e.target.value)
                if (joinError) setJoinError('')
              }}
              onJoinSubmit={handleJoinSubmit}
              onCreate={handleCreate}
              createLoading={createLoading}
              createStatus={createStatus}
            />
          )}
          {screen === 'waiting' && (
            <WaitingScreen
              code={roomCode}
              connectStartedRef={wsConnectStartedRef}
              roomWsRef={roomWsRef}
              onCancel={handleCancelWaiting}
              onReady={handlePlayer1Ready}
              addEvent={addEvent}
            />
          )}
          {screen === 'joining' && (
            <JoiningScreen
              code={roomCode}
              playerNum={playerNum}
              connectStartedRef={wsConnectStartedRef}
              onSuccess={handleJoinSuccess}
              onFailure={handleJoinFailure}
            />
          )}
          {screen === 'chat' && (
            <ChatRoom
              roomCode={roomCode}
              playerNum={playerNum}
              existingWs={roomWsRef.current}
              addEvent={addEvent}
              onEnd={handleChatEnd}
            />
          )}
        </div>
        <EventLog events={events} />
      </main>
    </div>
  )
}
