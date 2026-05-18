Criar negociação

# Criar negociação

Criar negociação

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
    "/deals": {
      "post": {
        "summary": "Criar negociação",
        "operationId": "crm-v1-create-deal",
        "tags": [
          "crm-v1-deals"
        ],
        "description": "Criar negociação",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "campaign": {
                    "type": "object",
                    "properties": {
                      "_id": {
                        "type": "string",
                        "example": "63d83e4f8cc2e6000f8e91c8"
                      }
                    }
                  },
                  "contacts": {
                    "type": "array",
                    "items": {
                      "type": "object",
                      "properties": {
                        "birthday": {
                          "type": "object",
                          "properties": {
                            "day": {
                              "type": "number",
                              "example": 11
                            },
                            "month": {
                              "type": "number",
                              "example": 9
                            },
                            "year": {
                              "type": "number",
                              "example": 1979
                            }
                          }
                        },
                        "emails": {
                          "type": "array",
                          "items": {
                            "type": "object",
                            "properties": {
                              "email": {
                                "type": "string",
                                "example": "email1@empresa.com"
                              }
                            }
                          }
                        },
                        "facebook": {
                          "type": "string",
                          "example": "https://..."
                        },
                        "legal_bases": {
                          "type": "array",
                          "items": {
                            "type": "object",
                            "properties": {
                              "category": {
                                "type": "string",
                                "example": "data_processing"
                              },
                              "status": {
                                "type": "string",
                                "example": "granted"
                              },
                              "type": {
                                "type": "string",
                                "example": "consent"
                              }
                            }
                          }
                        },
                        "linkedin": {
                          "type": "string",
                          "example": "http://..."
                        },
                        "name": {
                          "type": "string",
                          "example": "Alexandre"
                        },
                        "phones": {
                          "type": "array",
                          "items": {
                            "type": "object",
                            "properties": {
                              "phone": {
                                "type": "string",
                                "example": "3165585457"
                              },
                              "type": {
                                "type": "string",
                                "example": "cellphone"
                              }
                            }
                          }
                        },
                        "skype": {
                          "type": "string",
                          "example": "alexandre@email.com"
                        },
                        "title": {
                          "type": "string",
                          "example": "Gerente"
                        }
                      }
                    }
                  },
                  "deal": {
                    "type": "object",
                    "properties": {
                      "deal_custom_fields": {
                        "type": "array",
                        "items": {
                          "type": "object",
                          "properties": {
                            "custom_field_id": {
                              "type": "string",
                              "example": "63c188a315b034000fbf62d1"
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
                      "deal_stage_id": {
                        "type": "string",
                        "example": "633de55c96c4ac0019c8a638"
                      },
                      "name": {
                        "type": "string",
                        "example": "Negociação"
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
                      "user_id": {
                        "type": "string",
                        "example": "5d1e122a572a6c0034f2e595"
                      }
                    }
                  },
                  "deal_products": {
                    "type": "array",
                    "items": {
                      "type": "object",
                      "properties": {
                        "amount": {
                          "type": "number",
                          "example": 1.1
                        },
                        "base_price": {
                          "type": "number",
                          "example": 199.91
                        },
                        "description": {
                          "type": "string",
                          "example": "Description Product 1"
                        },
                        "discount_type": {
                          "type": "string",
                          "example": "value"
                        },
                        "name": {
                          "type": "string",
                          "example": "Product 01"
                        },
                        "price": {
                          "type": "number",
                          "example": 199.91
                        },
                        "recurrence": {
                          "type": "string",
                          "example": "spare"
                        },
                        "total": {
                          "type": "number",
                          "example": 199.91
                        }
                      }
                    }
                  },
                  "deal_source": {
                    "type": "object",
                    "properties": {
                      "_id": {
                        "type": "string",
                        "example": "61140ea5c794c70001b71985"
                      }
                    }
                  },
                  "distribution_settings": {
                    "type": "object",
                    "properties": {
                      "owner": {
                        "type": "object",
                        "properties": {
                          "email": {
                            "type": "string",
                            "example": "contato@email.com"
                          },
                          "id": {
                            "type": "string",
                            "example": "5d1e122a572a6c0034f2e595"
                          },
                          "type": {
                            "type": "string",
                            "example": "team"
                          }
                        }
                      }
                    }
                  },
                  "organization": {
                    "type": "object",
                    "properties": {
                      "_id": {
                        "type": "string",
                        "example": "64d3eb42e42434000114c68b"
                      }
                    }
                  }
                }
              },
              "examples": {
                "Criar negociação": {
                  "value": {
                    "campaign": {
                      "_id": "63d83e4f8cc2e6000f8e91c8"
                    },
                    "contacts": [
                      {
                        "birthday": {
                          "day": 11,
                          "month": 9,
                          "year": 1979
                        },
                        "emails": [
                          {
                            "email": "email1@empresa.com"
                          },
                          {
                            "email": "email2@empresa.com"
                          }
                        ],
                        "facebook": "https://...",
                        "legal_bases": [
                          {
                            "category": "data_processing",
                            "status": "granted",
                            "type": "consent"
                          },
                          {
                            "category": "communications",
                            "status": "granted",
                            "type": "vital_interest"
                          }
                        ],
                        "linkedin": "http://...",
                        "name": "Alexandre",
                        "phones": [
                          {
                            "phone": "3165585457",
                            "type": "cellphone"
                          }
                        ],
                        "skype": "alexandre@email.com",
                        "title": "Gerente"
                      }
                    ],
                    "deal": {
                      "deal_custom_fields": [
                        {
                          "custom_field_id": "63c188a315b034000fbf62d1",
                          "value": "Opção A"
                        },
                        {
                          "custom_field_id": "63e55e890ef6e0001819ddee",
                          "value": [
                            "Opção 1",
                            "Opção 2"
                          ]
                        }
                      ],
                      "deal_stage_id": "633de55c96c4ac0019c8a638",
                      "name": "Negociação",
                      "prediction_date": null,
                      "rating": 1,
                      "user_id": "5d1e122a572a6c0034f2e595"
                    },
                    "deal_products": [
                      {
                        "amount": 1.1,
                        "base_price": 199.91,
                        "description": "Description Product 1",
                        "discount_type": "value",
                        "name": "Product 01",
                        "price": 199.91,
                        "recurrence": "spare",
                        "total": 199.91
                      },
                      {
                        "amount": 1.1,
                        "base_price": 199.92,
                        "description": "Description Product 2",
                        "discount_type": "value",
                        "name": "Product 02",
                        "price": 199.92,
                        "recurrence": "spare",
                        "total": 199.92
                      }
                    ],
                    "deal_source": {
                      "_id": "61140ea5c794c70001b71985"
                    },
                    "distribution_settings": {
                      "owner": {
                        "email": "contato@email.com",
                        "id": "5d1e122a572a6c0034f2e595",
                        "type": "team"
                      }
                    },
                    "organization": {
                      "_id": "64d3eb42e42434000114c68b"
                    }
                  }
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
                      "example": 0
                    },
                    "amount_total": {
                      "type": "number",
                      "example": 399.83
                    },
                    "amount_unique": {
                      "type": "number",
                      "example": 399.83
                    },
                    "best_moment_to_touch": {
                      "type": "boolean",
                      "example": true
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
                          "example": "Campanha A"
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
                      "example": "2023-08-30T22:43:58.453-03:00"
                    },
                    "deal_custom_fields": {
                      "type": "array",
                      "items": {
                        "type": "object",
                        "properties": {
                          "created_at": {
                            "type": [
                              "string",
                              "null"
                            ],
                            "format": "date-time"
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
                                "example": true
                              },
                              "created_at": {
                                "type": "string",
                                "example": "2023-01-13T13:36:51.156-03:00"
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
                                "example": "Assuntos relacionados"
                              },
                              "opts": {
                                "type": "array",
                                "items": {
                                  "type": "string",
                                  "example": "Assunto 2"
                                },
                                "example": [
                                  "Assunto 2",
                                  "Assunto 3",
                                  "Opção A"
                                ]
                              },
                              "order": {
                                "type": "number",
                                "example": 1
                              },
                              "required": {
                                "type": "boolean",
                                "example": false
                              },
                              "type": {
                                "type": "string",
                                "example": "option"
                              },
                              "unique": {
                                "type": "boolean",
                                "example": false
                              },
                              "updated_at": {
                                "type": "string",
                                "example": "2023-08-30T21:51:06.034-03:00"
                              },
                              "visible": {
                                "type": "boolean",
                                "example": false
                              }
                            }
                          },
                          "custom_field_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "updated_at": {
                            "type": [
                              "string",
                              "null"
                            ],
                            "format": "date-time"
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
                    "deal_products": {
                      "type": "array",
                      "items": {
                        "type": "object",
                        "properties": {
                          "_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "amount": {
                            "type": "number",
                            "example": 1.1
                          },
                          "base_price": {
                            "type": "number",
                            "example": 199.91
                          },
                          "created_at": {
                            "type": "string",
                            "example": "2023-08-30T22:43:58.644-03:00"
                          },
                          "description": {
                            "type": "string",
                            "example": "Description Product 1"
                          },
                          "discount": {
                            "type": "number",
                            "example": 0
                          },
                          "discount_type": {
                            "type": "string",
                            "example": "value"
                          },
                          "id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "name": {
                            "type": "string",
                            "example": "Product 01"
                          },
                          "price": {
                            "type": "number",
                            "example": 199.91
                          },
                          "product_id": {
                            "type": "string",
                            "example": "ID"
                          },
                          "recurrence": {
                            "type": "string",
                            "example": "spare"
                          },
                          "total": {
                            "type": "number",
                            "example": 199.91
                          },
                          "updated_at": {
                            "type": "string",
                            "example": "2023-08-30T22:43:58.644-03:00"
                          }
                        }
                      }
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
                          "example": "3.0"
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
                          "example": "Converteu"
                        },
                        "nickname": {
                          "type": "string",
                          "example": "C"
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
                            "example": "2023-08-30T22:43:58.449-03:00"
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
                      "example": false
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
                      "example": "Negociação"
                    },
                    "organization": {
                      "type": "object",
                      "properties": {
                        "_id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "address": {
                          "type": [
                            "string",
                            "null"
                          ]
                        },
                        "address_latitude": {
                          "type": [
                            "string",
                            "null"
                          ]
                        },
                        "address_longitude": {
                          "type": [
                            "string",
                            "null"
                          ]
                        },
                        "id": {
                          "type": "string",
                          "example": "ID"
                        },
                        "name": {
                          "type": "string",
                          "example": "A2 Tecnologia & Inovação"
                        },
                        "organization_custom_fields": {
                          "type": "array",
                          "items": {}
                        },
                        "organization_segments": {
                          "type": "array",
                          "items": {}
                        },
                        "resume": {
                          "type": [
                            "string",
                            "null"
                          ]
                        },
                        "url": {
                          "type": [
                            "string",
                            "null"
                          ]
                        },
                        "visible": {
                          "type": "boolean",
                          "example": true
                        }
                      }
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
                          "example": "2026-05-25T22:43:58.453-03:00"
                        },
                        "expired": {
                          "type": "boolean",
                          "example": false
                        },
                        "expired_days": {
                          "type": "number",
                          "example": 0
                        }
                      }
                    },
                    "updated_at": {
                      "type": "string",
                      "example": "2023-08-30T22:43:58.677-03:00"
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
                      "amount_total": 399.83,
                      "amount_unique": 399.83,
                      "best_moment_to_touch": true,
                      "c_cf_errors": {},
                      "campaign": {
                        "_id": "ID",
                        "id": "ID",
                        "name": "Campanha A"
                      },
                      "campaign_id": "ID",
                      "closed_at": null,
                      "contact_errors": {},
                      "created_at": "2023-08-30T22:43:58.453-03:00",
                      "deal_custom_fields": [
                        {
                          "created_at": null,
                          "custom_field": {
                            "_id": "ID",
                            "allow_new": true,
                            "created_at": "2023-01-13T13:36:51.156-03:00",
                            "for": "deal",
                            "instance_id": "ID",
                            "label": "Assuntos relacionados",
                            "opts": [
                              "Assunto 2",
                              "Assunto 3",
                              "Opção A"
                            ],
                            "order": 1,
                            "required": false,
                            "type": "option",
                            "unique": false,
                            "updated_at": "2023-08-30T21:51:06.034-03:00",
                            "visible": false
                          },
                          "custom_field_id": "ID",
                          "updated_at": null,
                          "value": "Opção A"
                        },
                        {
                          "created_at": null,
                          "custom_field": {
                            "_id": "ID",
                            "allow_new": true,
                            "created_at": "2023-02-09T17:58:49.592-03:00",
                            "for": "deal",
                            "instance_id": "ID",
                            "label": "Tags",
                            "opts": [
                              "tag 1",
                              "tag 2",
                              "nome da tag",
                              "leads",
                              "[\"tag 1\"]",
                              "Opção 6 & 6",
                              "C 2",
                              "c 1",
                              "ana banana da silva",
                              "Opção 1",
                              "Opção 2"
                            ],
                            "order": 3,
                            "required": false,
                            "type": "multiple_choice",
                            "unique": false,
                            "updated_at": "2023-08-30T21:50:44.612-03:00",
                            "visible": false
                          },
                          "custom_field_id": "ID",
                          "updated_at": null,
                          "value": [
                            "Opção 1",
                            "Opção 2"
                          ]
                        }
                      ],
                      "deal_lost_reason_id": null,
                      "deal_products": [
                        {
                          "_id": "ID",
                          "amount": 1,
                          "amount_decimal": 1.1,
                          "base_price": 199.91,
                          "created_at": "2023-08-30T22:43:58.644-03:00",
                          "description": "Description Product 1",
                          "discount": 0,
                          "discount_type": "value",
                          "id": "ID",
                          "name": "Product 01",
                          "price": 199.91,
                          "product_id": "ID",
                          "recurrence": "spare",
                          "total": 199.91,
                          "updated_at": "2023-08-30T22:43:58.644-03:00"
                        },
                        {
                          "_id": "ID",
                          "amount": 1,
                          "amount_decimal": 1.1,
                          "base_price": 199.92,
                          "created_at": "2023-08-30T22:43:58.673-03:00",
                          "description": "Description Product 2",
                          "discount": 0,
                          "discount_type": "value",
                          "id": "ID",
                          "name": "Product 02",
                          "price": 199.92,
                          "product_id": "ID",
                          "recurrence": "spare",
                          "total": 199.92,
                          "updated_at": "2023-08-30T22:43:58.673-03:00"
                        }
                      ],
                      "deal_source": {
                        "_id": "ID",
                        "id": "ID",
                        "name": "3.0"
                      },
                      "deal_source_id": "ID",
                      "deal_stage": {
                        "_id": "ID",
                        "deal_pipeline_id": "ID",
                        "id": "ID",
                        "name": "Converteu",
                        "nickname": "C"
                      },
                      "deal_stage_histories": [
                        {
                          "_id": "ID",
                          "deal_stage_id": "ID",
                          "end_date": null,
                          "id": "ID",
                          "start_date": "2023-08-30T22:43:58.449-03:00"
                        },
                        {
                          "_id": "ID",
                          "deal_stage_id": "ID",
                          "end_date": null,
                          "id": "ID",
                          "start_date": "2023-08-30T22:43:58.455-03:00"
                        }
                      ],
                      "errors": {},
                      "from_rdsm_integration": false,
                      "hold": null,
                      "id": "ID",
                      "interactions": 0,
                      "last_note_content": null,
                      "name": "Negociação",
                      "organization": {
                        "_id": "ID",
                        "address": null,
                        "address_latitude": null,
                        "address_longitude": null,
                        "id": "ID",
                        "name": "A2 Tecnologia & Inovação",
                        "organization_custom_fields": [],
                        "organization_segments": [],
                        "resume": null,
                        "url": null,
                        "visible": true
                      },
                      "prediction_date": null,
                      "rating": 1,
                      "stop_time_limit": {
                        "expiration_date_time": "2026-05-25T22:43:58.453-03:00",
                        "expired": false,
                        "expired_days": 0
                      },
                      "updated_at": "2023-08-30T22:43:58.677-03:00",
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