export default function HomeScreen({
  joinInput,
  joinError,
  joinLoading,
  onJoinInputChange,
  onJoinSubmit,
  onCreate,
  createLoading,
}) {
  function handleSubmit(e) {
    e.preventDefault()
    onJoinSubmit()
  }

  return (
    <div className="home-screen">
      <div className="home-card">
        <h2 className="home-title">Start a chat</h2>
        <p className="home-desc">
          Create a new room and share the code, or enter a code to join a friend's room.
        </p>

        <button
          className="btn btn--primary home-create"
          type="button"
          onClick={onCreate}
          disabled={createLoading}
        >
          {createLoading ? 'Creating…' : 'Create New Chat'}
        </button>

        <div className="home-divider"><span>or</span></div>

        <form className="home-join" onSubmit={handleSubmit}>
          <input
            className="chatbox-input"
            type="text"
            placeholder="Enter 6-character code"
            value={joinInput}
            onChange={onJoinInputChange}
            maxLength={6}
            autoComplete="off"
            style={{ textTransform: 'uppercase', letterSpacing: '0.15em' }}
            disabled={joinLoading}
          />
          <button className="btn btn--secondary" type="submit" disabled={joinLoading}>
            {joinLoading ? 'Checking…' : 'Join Chat'}
          </button>
          {joinError && <p className="home-error" role="alert">{joinError}</p>}
        </form>
      </div>
    </div>
  )
}
