package com.lizongying.mytv

import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.GestureDetector
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.View
import android.view.ViewGroup
import android.view.View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
import android.view.WindowManager
import android.widget.Toast
import androidx.fragment.app.FragmentActivity
import androidx.leanback.widget.ImageCardView
import com.lizongying.mytv.models.TVViewModel


class MainActivity : FragmentActivity() {

    private val readyFragments = mutableSetOf<String>()
    private var playbackStarted = false
    private lateinit var playerFragment: PlayerFragment
    private lateinit var mainFragment: MainFragment
    private lateinit var infoFragment: InfoFragment
    private lateinit var settingFragment: SettingFragment

    private var doubleBackToExitPressedOnce = false

    private lateinit var gestureDetector: GestureDetector

    private val handler = Handler()
    private val delayHideMain: Long = 10000
    private val delayHideSetting: Long = 10000
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

        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        window.addFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN)
        window.decorView.systemUiVisibility = SYSTEM_UI_FLAG_HIDE_NAVIGATION

        val fragmentTransaction = supportFragmentManager.beginTransaction()
        if (savedInstanceState == null) {
            fragmentTransaction
                .add(R.id.main_browse_fragment, playerFragment)
                .add(R.id.main_browse_fragment, infoFragment)
                .add(R.id.main_browse_fragment, mainFragment)
        }
        fragmentTransaction.hide(mainFragment).commit()
        gestureDetector = GestureDetector(this, GestureListener())

    }

    fun showInfoFragment(tvViewModel: TVViewModel) {
        infoFragment.show(tvViewModel)
    }

    fun reloadChannels(url: String, onResult: (Boolean) -> Unit) =
        mainFragment.reloadChannels(url, onResult)

    fun settingHoldOpen() {
        handler.removeCallbacks(hideSetting)
    }

    fun play(tvViewModel: TVViewModel) {
        playerFragment.play(tvViewModel)
        mainFragment.view?.requestFocus()
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

    fun switchMainFragment() {
        val transaction = supportFragmentManager.beginTransaction()

        if (mainFragment.isHidden) {
            transaction.show(mainFragment)
            mainActive()
        } else {
            hideMainFragment()
            return
        }

        transaction.commit()
    }

    fun mainActive() {
        handler.removeCallbacks(hideMain)
        handler.postDelayed(hideMain, delayHideMain)
    }

    fun settingDelayHide() {
        handler.removeCallbacks(hideSetting)
        handler.postDelayed(hideSetting, delayHideSetting)
    }

    private val hideMain = Runnable {
        if (!mainFragment.isHidden) {
            supportFragmentManager.beginTransaction().hide(mainFragment).commit()
        }
    }

    private fun hideMainFragment() {
        if (!mainFragment.isHidden) {
            handler.removeCallbacks(hideMain)
            supportFragmentManager.beginTransaction()
                .hide(mainFragment)
                .commit()
        }
    }

    fun hideChannelList() {
        hideMainFragment()
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

    override fun dispatchTouchEvent(event: MotionEvent): Boolean {
        gestureDetector.onTouchEvent(event)
        return super.dispatchTouchEvent(event)
    }

    private inner class GestureListener : GestureDetector.SimpleOnGestureListener() {

        override fun onSingleTapConfirmed(e: MotionEvent): Boolean {
            if (mainFragment.isHidden) {
                switchMainFragment()
            } else if (!isPointOnChannelCard(mainFragment.view, e.rawX, e.rawY)) {
                hideMainFragment()
            }
            return true
        }

        override fun onDoubleTap(e: MotionEvent): Boolean {
            showSetting()
            return true
        }

        override fun onFling(
            e1: MotionEvent?,
            e2: MotionEvent,
            velocityX: Float,
            velocityY: Float
        ): Boolean {
            if (velocityY > 0) {
                if (mainFragment.isHidden) {
                    prev()
                }
            }
            if (velocityY < 0) {
                if (mainFragment.isHidden) {
                    next()
                }
            }
            return super.onFling(e1, e2, velocityX, velocityY)
        }
    }

    private fun isPointOnChannelCard(view: View?, rawX: Float, rawY: Float): Boolean {
        if (view == null || view.visibility != View.VISIBLE) return false

        if (view is ImageCardView) {
            val bounds = android.graphics.Rect()
            return view.getGlobalVisibleRect(bounds) && bounds.contains(rawX.toInt(), rawY.toInt())
        }

        if (view is ViewGroup) {
            for (index in 0 until view.childCount) {
                if (isPointOnChannelCard(view.getChildAt(index), rawX, rawY)) return true
            }
        }
        return false
    }

    private fun showSetting() {
        if (!mainFragment.isHidden) {
            return
        }

        Log.i(TAG, "settingFragment ${settingFragment.isVisible}")
        if (!settingFragment.isVisible) {
            settingFragment.show(supportFragmentManager, "setting")
            settingDelayHide()
        } else {
            handler.removeCallbacks(hideSetting)
            settingFragment.dismiss()
        }
    }

    private val hideSetting = Runnable {
        if (settingFragment.isVisible) {
            settingFragment.dismiss()
        }
    }

    private fun channelUp() {
        if (mainFragment.isHidden) {
            if (SP.channelReversal) {
                next()
                return
            }
            prev()
        }
    }

    private fun channelDown() {
        if (mainFragment.isHidden) {
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
        if (!mainFragment.isHidden) {
            hideMainFragment()
            return
        }

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
                switchMainFragment()
            }

            KeyEvent.KEYCODE_DPAD_CENTER -> {
                switchMainFragment()
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
        if (!mainFragment.isHidden) {
            handler.postDelayed(hideMain, delayHideMain)
        }
    }

    override fun onPause() {
        Log.i(TAG, "onPause")
        super.onPause()
        cancelChannelNumberEntry()
        playerFragment.hideChannelNumber()
        handler.removeCallbacks(hideMain)
        handler.removeCallbacks(hideSetting)
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
