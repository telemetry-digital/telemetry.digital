idf_component_register(
    SRCS "main.c"
    INCLUDE_DIRS "." "../../../examples/esp-idf/fridge_node/main"
    REQUIRES esp-idf nvs_flash esp_wifi esp_event esp_netif
)
