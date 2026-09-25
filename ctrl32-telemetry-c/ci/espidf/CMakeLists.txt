cmake_minimum_required(VERSION 3.16)
# The library port (ports/esp-idf) is an ESP-IDF component; PlatformIO passes src/ as the main component through
# EXTRA_COMPONENT_DIRS, so append instead of overriding.
list(APPEND EXTRA_COMPONENT_DIRS "${CMAKE_CURRENT_LIST_DIR}/../../ports")
include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(fridge_node)
