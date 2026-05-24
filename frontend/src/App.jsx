import { useState, useCallback, useRef } from 'react'
import ChatRoom from './components/ChatRoom'
import EventLog from './components/EventLog'
import HomeScreen from './screens/HomeScreen'
import WaitingScreen from './screens/WaitingScreen'
import JoiningScreen from './screens/JoiningScreen'
import { createRoom, validateRoom } from './api/rooms'
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

  const roomWsRef = useRef(null)
  const wsConnectStartedRef = useRef(false)

  const { theme, toggleTheme } = useTheme()
  const { events, addEvent } = useEventLog()

  const resetSession = useCallback(() => {
    wsConnectStartedRef.current = false
    setRoomCode('')
    setPlayerNum(null)
  }, [])

  const handleCreate = useCallback(async () => {
    setCreateLoading(true)
    try {
      const code = await createRoom()
      setRoomCode(code)
      setPlayerNum(1)
      wsConnectStartedRef.current = false
      setScreen('waiting')
      addEvent(`Room ${code} created - share the code with your friend`, 'success')
    } catch (err) {
      addEvent('Could not create room: ' + err.message, 'error')
    } finally {
      setCreateLoading(false)
    }
  }, [addEvent])

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
    try {
      await validateRoom(code, 2)
    } catch (err) {
      setJoinError(err.message)
      addEvent(err.message, 'error')
      return
    } finally {
      setJoinLoading(false)
    }

    setRoomCode(code)
    setPlayerNum(2)
    wsConnectStartedRef.current = false
    setScreen('joining')
    addEvent(`Joining room ${code}...`, 'info')
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
