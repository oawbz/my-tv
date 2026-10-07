package com.lizongying.mytv

import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.KeyEvent
import android.view.View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
import android.view.WindowManager
import android.widget.Toast
import androidx.fragment.app.FragmentActivity
import com.lizongying.mytv.models.TVViewModel


class MainActivity : FragmentActivity() {

    private val readyFragments = mutableSetOf<String>()
    private var playbackStarted = false
    private lateinit var playerFragment: PlayerFragment
    private lateinit var mainFragment: MainFragment
    private lateinit var infoFragment: InfoFragment
    private lateinit var settingFragment: SettingFragment
    private lateinit var channelControls: ChannelControls

    private var doubleBackToExitPressedOnce = false


    private val handler = Handler()
    private val channelListTimeoutMillis: Long = 10000
    private var channelNumberInput = ""

    private val channelNumberTimeout = Runnable {
        val number = channelNumberInput.toIntOrNull() ?: return@Runnable
        channelNumberInput = ""
        mainFragment.selectChannelNumber(number)
        playerFragment.showChannelNumber(number.toString(), CHANNEL_NUMBER_DISPLAY_MILLIS)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        Log.i(TAG, "onCreate")
        super.onCreate(savedInstanceState)

        val restored = supportFragmentManager.fragments
        playerFragment = restored.filterIsInstance<PlayerFragment>().firstOrNull() ?: PlayerFragment()
        mainFragment = restored.filterIsInstance<MainFragment>().firstOrNull() ?: MainFragment()
        infoFragment = restored.filterIsInstance<InfoFragment>().firstOrNull() ?: InfoFragment()
        settingFragment = restored.filterIsInstance<SettingFragment>().firstOrNull() ?: SettingFragment()

        setContentView(R.layout.activity_main)
        channelControls = ChannelControls(this, findViewById(R.id.channel_controls))

        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        window.addFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN)
        window.decorView.systemUiVisibility = SYSTEM_UI_FLAG_HIDE_NAVIGATION

        val fragmentTransaction = supportFragmentManager.beginTransaction()
        if (savedInstanceState == null) {
            fragmentTransaction
                .add(R.id.player_container, playerFragment)
                .add(R.id.player_container, infoFragment)
                .add(mainFragment, "channels")
        }
        fragmentTransaction.commit()
        channelControls.enableControls()

    }

    fun showInfoFragment(tvViewModel: TVViewModel) {
        infoFragment.setPlaybackActive(false)
        infoFragment.show(tvViewModel)
    }

    fun setPlaybackActive(active: Boolean) {
        infoFragment.setPlaybackActive(active)
    }

    fun reloadChannels(url: String, onResult: (Boolean) -> Unit) =
        mainFragment.reloadChannels(url, onResult)

    fun play(tvViewModel: TVViewModel) {
        playerFragment.play(tvViewModel)
        channelControls.setChannel(tvViewModel)
    }

    fun updateChannelGroups(groups: List<Pair<String, List<TVViewModel>>>) {
        channelControls.setGroups(groups)
    }

    fun playChannelFromList(channel: TVViewModel) {
        mainFragment.playChannelFromList(channel)
    }

    fun openSettings() {
        showSetting()
    }

    fun restartPlaybackEngine() {
        playerFragment.restartEngine()
    }

    fun showChannelSwitching() {
        playerFragment.showSwitching()
    }

    fun prev() {
        mainFragment.prev()
    }

    fun next() {
        mainFragment.next()
    }

    fun toggleChannelList() {
        if (channelControls.isDrawerOpen()) closeChannelList() else showChannelList()
    }

    fun showChannelList() {
        channelControls.openDrawer()
        channelListActive()
    }

    fun channelListActive() {
        handler.removeCallbacks(hideChannelListTimeout)
        handler.postDelayed(hideChannelListTimeout, channelListTimeoutMillis)
    }

    private val hideChannelListTimeout = Runnable { channelControls.closeAll() }

    private fun closeChannelList() {
        handler.removeCallbacks(hideChannelListTimeout)
        channelControls.closeAll()
    }

    fun hideChannelList() {
        closeChannelList()
    }

    fun fragmentReady(tag: String) {
        readyFragments.add(tag)
        Log.i(TAG, "ready $tag")
        if (readyFragments.containsAll(REQUIRED_FRAGMENTS) && !playbackStarted) {
            playbackStarted = true
            mainFragment.fragmentReady()
        }
    }

    fun fragmentUnavailable(tag: String) {
        readyFragments.remove(tag)
        playbackStarted = false
    }

    override fun dispatchKeyEvent(event: KeyEvent): Boolean {
        if (channelControls.isDrawerOpen() && !settingFragment.isVisible) {
            val navigation = event.keyCode in setOf(
                KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_DPAD_DOWN,
                KeyEvent.KEYCODE_DPAD_LEFT, KeyEvent.KEYCODE_DPAD_RIGHT,
                KeyEvent.KEYCODE_DPAD_CENTER, KeyEvent.KEYCODE_ENTER,
            )
            if (navigation) {
                if (event.action == KeyEvent.ACTION_DOWN &&
                    (event.repeatCount == 0 || event.keyCode !in setOf(
                        KeyEvent.KEYCODE_DPAD_CENTER, KeyEvent.KEYCODE_ENTER
                    ))
                ) channelControls.handleRemoteKey(event.keyCode)
                return true
            }
        }
        return super.dispatchKeyEvent(event)
    }

    private fun showSetting() {
        if (supportFragmentManager.isStateSaved || isFinishing) return
        channelControls.closeAll()

        Log.i(TAG, "settingFragment ${settingFragment.isVisible}")
        if (!settingFragment.isVisible) {
            settingFragment.showNow(supportFragmentManager, "setting")
        } else {
            settingFragment.dismiss()
        }
    }

    private fun channelUp() {
        if (!channelControls.isDrawerOpen()) {
            if (SP.channelReversal) {
                next()
                return
            }
            prev()
        }
    }

    private fun channelDown() {
        if (!channelControls.isDrawerOpen()) {
            if (SP.channelReversal) {
                prev()
                return
            }
            next()
        }
    }

    private fun handleChannelDigit(digit: Int) {
        if (channelNumberInput.isEmpty()) {
            channelNumberInput = digit.toString()
            playerFragment.showChannelNumber("$channelNumberInput-")
            handler.removeCallbacks(channelNumberTimeout)
            handler.postDelayed(channelNumberTimeout, CHANNEL_NUMBER_ENTRY_TIMEOUT_MILLIS)
            return
        }

        handler.removeCallbacks(channelNumberTimeout)
        channelNumberInput += digit
        val number = channelNumberInput.toIntOrNull()
        channelNumberInput = ""
        if (number != null) mainFragment.selectChannelNumber(number)
        playerFragment.showChannelNumber(number?.toString() ?: "", CHANNEL_NUMBER_DISPLAY_MILLIS)
    }

    private fun cancelChannelNumberEntry() {
        if (channelNumberInput.isEmpty()) return
        handler.removeCallbacks(channelNumberTimeout)
        channelNumberInput = ""
        playerFragment.hideChannelNumber()
    }

    private fun confirmChannelNumberEntry(): Boolean {
        val number = channelNumberInput.toIntOrNull() ?: return false
        handler.removeCallbacks(channelNumberTimeout)
        channelNumberInput = ""
        mainFragment.selectChannelNumber(number)
        playerFragment.showChannelNumber(number.toString(), CHANNEL_NUMBER_DISPLAY_MILLIS)
        return true
    }

    private fun back() {
        if (channelControls.closeOverlay()) return

        if (doubleBackToExitPressedOnce) {
            super.onBackPressed()
            return
        }

        doubleBackToExitPressedOnce = true
        Toast.makeText(this, "再按一次退出", Toast.LENGTH_SHORT).show()

        Handler(Looper.getMainLooper()).postDelayed({
            doubleBackToExitPressedOnce = false
        }, 2000)
    }

    override fun onKeyDown(keyCode: Int, event: KeyEvent?): Boolean {
        Log.i(TAG, "keyCode $keyCode, event $event")
        val digit = keyCode.toChannelDigit()
        if (digit != null && SP.channelNumberInput) {
            if (event?.repeatCount == 0) handleChannelDigit(digit)
            return true
        }
        if (keyCode == KeyEvent.KEYCODE_ENTER || keyCode == KeyEvent.KEYCODE_DPAD_CENTER) {
            if (confirmChannelNumberEntry()) return true
        }
        cancelChannelNumberEntry()

        when (keyCode) {
            KeyEvent.KEYCODE_ESCAPE -> {
                back()
                return true
            }

            KeyEvent.KEYCODE_BACK -> {
                back()
                return true
            }

            KeyEvent.KEYCODE_BOOKMARK -> {
                showSetting()
                return true
            }

            KeyEvent.KEYCODE_UNKNOWN -> {
                showSetting()
                return true
            }

            KeyEvent.KEYCODE_HELP -> {
                showSetting()
                return true
            }

            KeyEvent.KEYCODE_SETTINGS -> {
                showSetting()
                return true
            }

            KeyEvent.KEYCODE_MENU -> {
                showSetting()
                return true
            }

            KeyEvent.KEYCODE_ENTER -> {
                if (!channelControls.isDrawerOpen()) {
                    toggleChannelList()
                } else {
                    channelControls.handleRemoteKey(keyCode)
                }
                return true
            }

            KeyEvent.KEYCODE_DPAD_CENTER -> {
                if (!channelControls.isDrawerOpen()) {
                    toggleChannelList()
                } else {
                    channelControls.handleRemoteKey(keyCode)
                }
                return true
            }

            KeyEvent.KEYCODE_DPAD_LEFT -> {
                return true
            }

            KeyEvent.KEYCODE_DPAD_RIGHT -> {
                return true
            }

            KeyEvent.KEYCODE_DPAD_UP -> {
                channelUp()
            }

            KeyEvent.KEYCODE_CHANNEL_UP -> {
                channelUp()
            }

            KeyEvent.KEYCODE_DPAD_DOWN -> {
                channelDown()
            }

            KeyEvent.KEYCODE_CHANNEL_DOWN -> {
                channelDown()
            }

        }

        return super.onKeyDown(keyCode, event)
    }

    override fun onStart() {
        Log.i(TAG, "onStart")
        super.onStart()
    }

    override fun onResume() {
        Log.i(TAG, "onResume")
        super.onResume()
        if (channelControls.isDrawerOpen()) {
            handler.postDelayed(hideChannelListTimeout, channelListTimeoutMillis)
        }
    }

    override fun onPause() {
        Log.i(TAG, "onPause")
        super.onPause()
        cancelChannelNumberEntry()
        playerFragment.hideChannelNumber()
        handler.removeCallbacks(hideChannelListTimeout)
        channelControls.closeAll()
    }

    private companion object {
        const val TAG = "MainActivity"
        const val CHANNEL_NUMBER_ENTRY_TIMEOUT_MILLIS = 1500L
        const val CHANNEL_NUMBER_DISPLAY_MILLIS = 1200L
        val REQUIRED_FRAGMENTS = setOf(
            "PlayerFragment", "MainFragment", "InfoFragment"
        )
    }
}

private fun Int.toChannelDigit(): Int? = when (this) {
    in KeyEvent.KEYCODE_0..KeyEvent.KEYCODE_9 -> this - KeyEvent.KEYCODE_0
    in KeyEvent.KEYCODE_NUMPAD_0..KeyEvent.KEYCODE_NUMPAD_9 -> this - KeyEvent.KEYCODE_NUMPAD_0
    else -> null
}
