import QtQuick
import Quickshell
import qs.Commons
import qs.Ui

// fogway's icon in Omarchy's bar, put there by fogway (Settings → Bar icon).
// A click drops down its panel under the bar, where the click was; a right
// click opens its window. fogway is started if it isn't running.
BarWidget {
  id: root
  moduleName: "fogway.fogway"

  // written in by fogway when it puts the widget here
  readonly property string fogway: "__FOGWAY__"

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: String.fromCodePoint(0xf15c6) // nf-md-bird
    tooltipText: "fogway"
    onPressed: function(b) {
      Quickshell.execDetached(b === Qt.RightButton ? [root.fogway, "app"] : [root.fogway, "panel"])
    }
  }
}
