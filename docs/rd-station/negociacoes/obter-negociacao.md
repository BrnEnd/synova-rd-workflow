Obter negociação

# Obter negociação

Obter negociação

# OpenAPI definition

```json
{
  "openapi": "3.1.1",
  "info": {
    "title": "RD Station CRM API v1",
    "version": "1.0",
    "description": "Documentação da API v1 do RD Station CRM.",
    "license": {
      "name": "Termos de Uso do Software RD Station CRM",
      "url": "https://legal.rdstation.com/pt/rdstation-crm-services-agreement/"
    },
    "termsOfService": "https://legal.rdstation.com/pt/rdstation-crm-services-agreement/",
    "contact": {
      "name": "Suporte RD Station",
      "url": "https://developers.rdstation.com/docs/suporte"
    }
  },
  "externalDocs": {
    "description": "Documentação completa",
    "url": "https://developers.rdstation.com/crm-v1"
  },
  "servers": [
    {
      "url": "https://crm.rdstation.com/api/v1",
      "description": "Production"
    }
  ],
  "security": [
    {
      "Token": []
    }
  ],
  "tags": [
    {
      "name": "crm-v1-deals",
      "description": "Negociações"
    }
  ],
  "paths": {
    "/deals/{deal_id}": {
      "get": {
        "summary": "Obter negociação",
        "operationId": "crm-v1-get-deal",
        "tags": [
          "crm-v1-deals"
        ],
        "description": "Obter negociação",
        "parameters": [
          {
            "name": "deal_id",
            "in": "path",
            "required": true,
            "description": "ID da negociação.",
            "schema": {
              "type": "string"
            }
          }
        ],
        "requestBody": {
          "content": {
            "application/json": {
              "examples": {
                "Obter negociação": {
                  "value": ""
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Success",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "_id": {
                      "type": "string",
                      "example": "ID"
                    },
                    "amount_montly": {
                      "type": "number",
                      "example": 1.1
                    },
                    "amount_total": {
                      "type": "number",
                      "example": 0
                    },
                    "amount_unique": {
                      "type": "number",
                      "example": 0
                    },
                    "best_moment_to_touch": {
                      "type": "boolean",
                      "example": false
                    },
                    "c_cf_errors": {
                      "type": "object",
                      "properties": {}
                    },
                    "campaign": {
                      "type": "object",
                      "properties": {
                        "_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "name": {
                          "type": "string",
                          "example": "Marcado como Oportunidade no RD Station Marketing"
                        }
                      }
                    },
                    "campaign_id": {
                      "type": "string",
                      "example": "ID"
                    },
                    "closed_at": {
                      "type": [
                        "string",
                        "null"
                      ],
                      "format": "date-time"
                    },
                    "contact_errors": {
                      "type": "object",
                      "properties": {}
                    },
                    "created_at": {
                      "type": "string",
                      "example": "2023-03-30T16:27:24.764-03:00"
                    },
                    "deal_custom_fields": {
                      "type": "array",
                      "items": {
                        "type": "object",
                        "properties": {
                          "created_at": {
                            "type": "string",
                            "example": "2023-03-30T16:23:54.781-03:00"
                          },
                          "custom_field": {
                            "type": "object",
                            "properties": {
                              "_id": {
                                "type": "string",
                                "example": "ID"
                              },
                              "allow_new": {
                                "type": "boolean",
                                "example": false
                              },
                              "created_at": {
                                "type": "string",
                                "example": "2020-08-20T17:32:35.618-03:00"
                              },
                              "for": {
                                "type": "string",
                                "example": "deal"
                              },
                              "instance_id": {
                                "type": "string",
                                "example": "ID"
                              },
                              "label": {
                                "type": "string",
                                "example": "Custom date field"
                              },
                              "opts": {
                                "type": "array",
                                "items": {}
                              },
                              "order": {
                                "type": "number",
                                "example": 8
                              },
                              "required": {
                                "type": "boolean",
                                "example": true
                              },
                              "required_rules": {
                                "type": "array",
                                "items": {
                                  "type": "object",
                                  "properties": {
                                    "_id": {
                                      "type": "string",
                                      "example": "ID"
                                    },
                                    "always": {
                                      "type": "boolean",
                                      "example": true
                                    },
                                    "created_at": {
                                      "type": "string",
                                      "example": "2023-03-30T10:52:55.465-03:00"
                                    },
                                    "property": {
                                      "type": [
                                        "string",
                                        "null"
                                      ]
                                    },
                                    "updated_at": {
                                      "type": "string",
                                      "example": "2023-03-30T10:52:55.465-03:00"
                                    },
                                    "value": {
                                      "type": [
                                        "string",
                                        "null"
                                      ]
                                    }
                                  }
                                }
                              },
                              "type": {
                                "type": "string",
                                "example": "date"
                              },
                              "unique": {
                                "type": "boolean",
                                "example": false
                              },
                              "updated_at": {
                                "type": "string",
                                "example": "2023-03-30T10:52:55.465-03:00"
                              },
                              "visible": {
                                "type": "boolean",
                                "example": true
                              }
                            }
                          },
                          "custom_field_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "updated_at": {
                            "type": "string",
                            "example": "2023-03-30T16:23:54.781-03:00"
                          },
                          "value": {
                            "example": "Opção A",
                            "anyOf": [
                              {
                                "type": "string",
                                "example": "Opção A"
                              },
                              {
                                "type": "array",
                                "items": {
                                  "type": "string",
                                  "example": "Opção 1"
                                }
                              }
                            ]
                          }
                        }
                      }
                    },
                    "deal_lost_reason_id": {
                      "type": [
                        "string",
                        "null"
                      ]
                    },
                    "deal_pipeline": {
                      "type": "object",
                      "properties": {
                        "id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "name": {
                          "type": "string",
                          "example": "Funil de Vendas 01"
                        }
                      }
                    },
                    "deal_pipeline_relation": {
                      "type": "object",
                      "properties": {
                        "parent_deal": {
                          "type": "object",
                          "properties": {
                            "amount_montly": {
                              "type": "number",
                              "example": 0
                            },
                            "amount_unique": {
                              "type": "number",
                              "example": 0
                            },
                            "closed_at": {
                              "type": "string",
                              "example": "2023-03-30T16:53:39.754-03:00"
                            },
                            "created_at": {
                              "type": "string",
                              "example": "2021-08-12T17:03:26.063-03:00"
                            },
                            "deal_pipeline": {
                              "type": "object",
                              "properties": {
                                "id": {
                                  "type": "string",
                                  "example": "ID"
                                },
                                "name": {
                                  "type": "string",
                                  "example": "Funil de vendas 3333"
                                }
                              }
                            },
                            "deal_stage": {
                              "type": "object",
                              "properties": {
                                "id": {
                                  "type": "string",
                                  "example": "ID"
                                },
                                "name": {
                                  "type": "string",
                                  "example": "Identificação do interesse"
                                }
                              }
                            },
                            "id": {
                              "type": "string",
                              "example": "ID"
                            },
                            "name": {
                              "type": "string",
                              "example": "NAME"
                            },
                            "prediction_date": {
                              "type": [
                                "string",
                                "null"
                              ],
                              "format": "date"
                            },
                            "user": {
                              "type": "object",
                              "properties": {
                                "id": {
                                  "type": "string",
                                  "example": "ID"
                                },
                                "name": {
                                  "type": "string",
                                  "example": "NAME"
                                }
                              }
                            }
                          }
                        }
                      }
                    },
                    "deal_products": {
                      "type": "array",
                      "items": {}
                    },
                    "deal_source": {
                      "type": "object",
                      "properties": {
                        "_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "name": {
                          "type": "string",
                          "example": "Desconhecido"
                        }
                      }
                    },
                    "deal_source_id": {
                      "type": "string",
                      "example": "ID"
                    },
                    "deal_stage": {
                      "type": "object",
                      "properties": {
                        "_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "deal_pipeline_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "name": {
                          "type": "string",
                          "example": "Proposta enviada"
                        },
                        "nickname": {
                          "type": "string",
                          "example": "PE"
                        }
                      }
                    },
                    "deal_stage_histories": {
                      "type": "array",
                      "items": {
                        "type": "object",
                        "properties": {
                          "_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "deal_stage_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "end_date": {
                            "type": [
                              "string",
                              "null"
                            ],
                            "format": "date-time"
                          },
                          "id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "start_date": {
                            "type": "string",
                            "example": "2023-03-30T16:27:24.759-03:00"
                          }
                        }
                      }
                    },
                    "errors": {
                      "type": "object",
                      "properties": {}
                    },
                    "from_rdsm_integration": {
                      "type": "boolean",
                      "example": true
                    },
                    "hold": {
                      "type": [
                        "boolean",
                        "null"
                      ]
                    },
                    "id": {
                      "type": "string",
                      "example": "ID"
                    },
                    "interactions": {
                      "type": "number",
                      "example": 0
                    },
                    "last_note_content": {
                      "type": [
                        "string",
                        "null"
                      ]
                    },
                    "name": {
                      "type": "string",
                      "example": "NAME"
                    },
                    "prediction_date": {
                      "type": [
                        "string",
                        "null"
                      ],
                      "format": "date"
                    },
                    "rating": {
                      "type": "number",
                      "example": 1
                    },
                    "stop_time_limit": {
                      "type": "object",
                      "properties": {
                        "expiration_date_time": {
                          "type": "string",
                          "example": "2023-03-31T16:27:24.764-03:00"
                        },
                        "expired": {
                          "type": "boolean",
                          "example": true
                        },
                        "expired_days": {
                          "type": "number",
                          "example": 9
                        }
                      }
                    },
                    "updated_at": {
                      "type": "string",
                      "example": "2023-03-30T16:27:33.819-03:00"
                    },
                    "user": {
                      "type": "object",
                      "properties": {
                        "_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "name": {
                          "type": "string",
                          "example": "NAME"
                        }
                      }
                    },
                    "win": {
                      "type": [
                        "boolean",
                        "null"
                      ]
                    }
                  }
                },
                "examples": {
                  "Success": {
                    "value": {
                      "_id": "ID",
                      "amount_montly": 0,
                      "amount_total": 0,
                      "amount_unique": 0,
                      "best_moment_to_touch": false,
                      "c_cf_errors": {},
                      "campaign": {
                        "_id": "ID",
                        "id": "ID",
                        "name": "Marcado como Oportunidade no RD Station Marketing"
                      },
                      "campaign_id": "ID",
                      "closed_at": null,
                      "contact_errors": {},
                      "created_at": "2023-03-30T16:27:24.764-03:00",
                      "deal_custom_fields": [
                        {
                          "created_at": "2023-03-30T16:23:54.781-03:00",
                          "custom_field": {
                            "_id": "ID",
                            "allow_new": false,
                            "created_at": "2020-08-20T17:32:35.618-03:00",
                            "for": "deal",
                            "instance_id": "ID",
                            "label": "Custom date field",
                            "opts": [],
                            "order": 8,
                            "required": true,
                            "required_rules": [
                              {
                                "_id": "ID",
                                "always": true,
                                "created_at": "2023-03-30T10:52:55.465-03:00",
                                "property": null,
                                "updated_at": "2023-03-30T10:52:55.465-03:00",
                                "value": null
                              }
                            ],
                            "type": "date",
                            "unique": false,
                            "updated_at": "2023-03-30T10:52:55.465-03:00",
                            "visible": true
                          },
                          "custom_field_id": "ID",
                          "updated_at": "2023-03-30T16:23:54.781-03:00",
                          "value": "31/03/2023"
                        }
                      ],
                      "deal_lost_reason_id": null,
                      "deal_pipeline": {
                        "id": "ID",
                        "name": "Funil de Vendas 01"
                      },
                      "deal_pipeline_relation": {
                        "parent_deal": {
                          "amount_montly": 0,
                          "amount_unique": 0,
                          "closed_at": "2023-03-30T16:53:39.754-03:00",
                          "created_at": "2021-08-12T17:03:26.063-03:00",
                          "deal_pipeline": {
                            "id": "ID",
                            "name": "Funil de vendas 3333"
                          },
                          "deal_stage": {
                            "id": "ID",
                            "name": "Identificação do interesse"
                          },
                          "id": "ID",
                          "name": "NAME",
                          "prediction_date": null,
                          "user": {
                            "id": "ID",
                            "name": "NAME"
                          }
                        }
                      },
                      "deal_products": [],
                      "deal_source": {
                        "_id": "ID",
                        "id": "ID",
                        "name": "Desconhecido"
                      },
                      "deal_source_id": "ID",
                      "deal_stage": {
                        "_id": "ID",
                        "deal_pipeline_id": "ID",
                        "id": "ID",
                        "name": "Proposta enviada",
                        "nickname": "PE"
                      },
                      "deal_stage_histories": [
                        {
                          "_id": "ID",
                          "deal_stage_id": "ID",
                          "end_date": null,
                          "id": "ID",
                          "start_date": "2023-03-30T16:27:24.759-03:00"
                        },
                        {
                          "_id": "ID",
                          "deal_stage_id": "54480f523f64f90155000034",
                          "end_date": null,
                          "id": "ID",
                          "start_date": "2023-03-30T16:27:24.767-03:00"
                        }
                      ],
                      "errors": {},
                      "from_rdsm_integration": true,
                      "hold": null,
                      "id": "ID",
                      "interactions": 0,
                      "last_note_content": null,
                      "name": "NAME",
                      "prediction_date": null,
                      "rating": 1,
                      "stop_time_limit": {
                        "expiration_date_time": "2023-03-31T16:27:24.764-03:00",
                        "expired": true,
                        "expired_days": 9
                      },
                      "updated_at": "2023-03-30T16:27:33.819-03:00",
                      "user": {
                        "_id": "ID",
                        "id": "ID",
                        "name": "NAME"
                      },
                      "win": null
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  },
  "components": {
    "securitySchemes": {
      "Token": {
        "type": "apiKey",
        "name": "token",
        "in": "query",
        "description": "Token do usuário"
      }
    }
  }
}
```