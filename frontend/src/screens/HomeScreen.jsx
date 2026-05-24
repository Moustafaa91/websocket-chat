export default function HomeScreen({
  joinInput,
  joinError,
  joinLoading,
  onJoinInputChange,
  onJoinSubmit,
  onCreate,
  createLoading,
  createStatus,
}) {
  function handleSubmit(e) {
    e.preventDefault()
    onJoinSubmit()
  }

  return (
    <div className="home-screen">
      <div className="home-card home-card--wide">
        <h2 className="home-title">Start a WebSocket chat</h2>
        <p className="home-desc">
          Create a room for a two-person session, or join one with a six-character code.
        </p>

        <section className="home-action">
          <div>
            <h3>Create a room</h3>
            <p>Generate a code and wait for the second participant to connect.</p>
          </div>
          <button
            className="btn btn--primary home-create"
            type="button"
            onClick={onCreate}
            disabled={createLoading}
          >
            {createLoading ? 'Waking server...' : 'Create New Chat'}
          </button>
          {createStatus && (
            <p className="home-status" role="status" aria-live="polite">
              <span className="waiting-spinner" aria-hidden="true" />
              <span>{createStatus}</span>
            </p>
          )}
        </section>

        <div className="home-divider"><span>or</span></div>

        <form className="home-action home-join" onSubmit={handleSubmit}>
          <div>
            <h3>Join a room</h3>
            <p>Enter the code shared by Player 1.</p>
          </div>
          <div className="join-row">
            <input
              className="chatbox-input join-code-input"
              type="text"
              placeholder="ABC123"
              value={joinInput}
              onChange={onJoinInputChange}
              maxLength={6}
              autoComplete="off"
              disabled={joinLoading}
              aria-label="Room code"
            />
            <button className="btn btn--secondary" type="submit" disabled={joinLoading}>
              {joinLoading ? 'Checking...' : 'Join Chat'}
            </button>
          </div>
          {joinError && <p className="home-error" role="alert">{joinError}</p>}
        </form>
      </div>
    </div>
  )
}
